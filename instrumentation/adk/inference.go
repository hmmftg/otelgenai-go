package adk

import (
	"time"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/safety"
)

// beforeModel creates model state for the agent key if no active model
// operation exists. On collision, reports a diagnostic and preserves
// existing state. Never mutates the generate_content span. Returns
// (nil, nil) to indicate no replacement.
//
// When content events are enabled, the resolved provider name and
// projected system instructions / input messages are stored on the
// model state for the terminal inference-details event. A pending
// child error on the agent produces a continued_after_error
// occurrence: the agent is observed starting a new model call after an
// earlier error, which is an observation, not a retry or recovery claim.
func (p *Plugin) beforeModel(ctx callbackContext, req *model.LLMRequest, occurredAt time.Time) {
	key := deriveAgentKey(ctx)

	if et, ok := p.registry.takePendingError(key); ok {
		p.instr.EmitAgentOccurrence(ctx, otelgenai.AgentOccurrence{
			Kind:           otelgenai.OccurrenceContinuedAfterError,
			AgentName:      ctx.AgentName(),
			Operation:      otelgenai.Operation("generate_content"),
			ConversationID: ctx.SessionID(),
			ErrorType:      et,
			OccurredAt:     occurredAt,
		})
	}

	modelName := ""
	if req != nil {
		modelName = req.Model
	}
	_, ok := p.registry.startModel(key, modelName)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureModelStateConflict)
		return
	}

	if !p.instr.ContentEventsEnabled(ctx) {
		return
	}
	provider, ok := p.resolveInferenceProvider(modelName)
	if !ok {
		return
	}
	p.registry.updateModelRequest(key, provider,
		p.projectSystemInstructions(req),
		p.projectInputMessages(req),
	)
}

// afterModel handles both partial and terminal model responses. For
// partial responses, it updates cumulative usage state only. For
// terminal responses, it emits duration, usage, and the
// inference-details event, and increments the parent agent inference
// count exactly once. Never mutates the generate_content span.
func (p *Plugin) afterModel(ctx callbackContext, resp *model.LLMResponse, llmResponseError error, occurredAt time.Time) {
	key := deriveAgentKey(ctx)
	st := p.registry.getModel(key)
	if st == nil {
		return
	}

	partial := false
	var usage *genai.GenerateContentResponseUsageMetadata
	if resp != nil {
		partial = resp.Partial
		usage = resp.UsageMetadata
	}
	if usage != nil {
		p.registry.updateModelUsage(key, mapUsageMetadata(usage), partial)
	} else if partial {
		p.registry.updateModelUsage(key, st.usage, true)
	}

	// Terminal condition: non-partial response or error.
	terminal := !partial || llmResponseError != nil
	if !terminal {
		return
	}

	// Terminalize once.
	termSt := p.registry.endModel(key)
	if termSt == nil {
		return
	}

	duration := time.Since(termSt.start)

	p.instr.RecordInferenceDuration(ctx, duration, p.system, termSt.model, otelgenai.Operation("generate_content"))
	p.instr.RecordInferenceUsage(ctx, termSt.usage, p.system, termSt.model, otelgenai.Operation("generate_content"))
	p.registry.incrementAgentInference(key)

	if llmResponseError != nil {
		et := p.instr.ClassifyError(llmResponseError)
		if et == otelgenai.ErrorTypeNone {
			et = otelgenai.ErrorTypeUnknown
		}
		p.registry.markPendingError(key, et)
	}

	// Emit the standard inference-details event when projected content
	// exists. The event is correlated to the enclosing agent trace
	// context because ADK v2.3.0 ends the generate_content span before
	// invoking AfterModel callbacks.
	if termSt.provider == "" {
		return
	}
	details := otelgenai.InferenceDetails{
		Operation:           otelgenai.Operation("generate_content"),
		Provider:            termSt.provider,
		RequestModel:        termSt.model,
		ConversationID:      ctx.SessionID(),
		Streaming:           termSt.streaming,
		Usage:               termSt.usage,
		SystemInstructions:  termSt.systemInstructions,
		InputMessages:       termSt.inputMessages,
		OccurredAt:          occurredAt,
	}
	if resp != nil {
		details.ResponseModel = resp.ModelVersion
		if fr := schemaFinishReason(resp, hasResponseToolCall(resp)); fr != "" {
			details.FinishReasons = []string{fr}
		}
		details.OutputMessages = p.projectOutputMessages(resp)
	}
	if llmResponseError != nil {
		et := p.instr.ClassifyError(llmResponseError)
		if et == otelgenai.ErrorTypeNone {
			et = otelgenai.ErrorTypeUnknown
		}
		details.ErrorType = et
	}
	p.instr.EmitInferenceDetails(ctx, details)
}

// onModelError records the observed model error as an occurrence event
// and marks the agent with a pending error so a subsequent
// BeforeModel/BeforeTool reports continued_after_error. It does not
// terminalize state or mutate the span; the terminal AfterModel error
// remains authoritative.
func (p *Plugin) onModelError(ctx callbackContext, err error, occurredAt time.Time) {
	if err == nil {
		return
	}
	key := deriveAgentKey(ctx)
	et := p.instr.ClassifyError(err)
	if et == otelgenai.ErrorTypeNone {
		et = otelgenai.ErrorTypeUnknown
	}
	p.registry.markPendingError(key, et)
	p.instr.EmitAgentOccurrence(ctx, otelgenai.AgentOccurrence{
		Kind:           otelgenai.OccurrenceModelErrorObserved,
		AgentName:      ctx.AgentName(),
		Operation:      otelgenai.Operation("generate_content"),
		ConversationID: ctx.SessionID(),
		ErrorType:      et,
		OccurredAt:     occurredAt,
	})
}

// resolveInferenceProvider resolves the gen_ai.provider.name for the
// standard inference-details event. A configured resolver takes
// precedence over the static provider name. Returns false when no
// provider can be determined; the standard event requires it and the
// adapter must not fabricate one.
func (p *Plugin) resolveInferenceProvider(modelName string) (string, bool) {
	if p.providerResolver != nil {
		var name string
		panicErr := safety.GuardedCall(func() error {
			name = p.providerResolver(modelName)
			return nil
		})
		if panicErr != nil {
			p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureProviderResolverPanic)
			return "", false
		}
		if name != "" {
			return name, true
		}
	}
	if p.provider != "" {
		return p.provider, true
	}
	return "", false
}

// hasResponseToolCall reports whether the response content carries a
// tool call part.
func hasResponseToolCall(resp *model.LLMResponse) bool {
	if resp == nil || resp.Content == nil {
		return false
	}
	for _, part := range resp.Content.Parts {
		if part != nil && part.FunctionCall != nil {
			return true
		}
	}
	return false
}

// mapUsageMetadata converts ADK's genai UsageMetadata to otelgenai.Usage.
//
//	InputTokens      = PromptTokenCount
//	OutputTokens     = CandidatesTokenCount + ThoughtsTokenCount
//	CacheReadTokens  = CachedContentTokenCount
//	CacheWriteTokens = 0
//	ReasoningTokens  = ThoughtsTokenCount
//
// ReasoningTokens is a subset of OutputTokens. Only input/output produce
// existing token histogram measurements.
func mapUsageMetadata(um *genai.GenerateContentResponseUsageMetadata) otelgenai.Usage {
	if um == nil {
		return otelgenai.Usage{}
	}
	return otelgenai.Usage{
		InputTokens:      int64(um.PromptTokenCount),
		OutputTokens:     int64(um.CandidatesTokenCount) + int64(um.ThoughtsTokenCount),
		CacheReadTokens:  int64(um.CachedContentTokenCount),
		CacheWriteTokens: 0,
		ReasoningTokens:  int64(um.ThoughtsTokenCount),
	}
}
