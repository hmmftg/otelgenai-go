package adk

import (
	"context"
	"time"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
)

// beforeModel creates model state for the agent key if no active model
// operation exists. On collision, reports a diagnostic and preserves
// existing state. Never mutates the generate_content span. Returns
// (nil, nil) to indicate no replacement.
func (p *Plugin) beforeModel(ctx callbackContext, modelName string) {
	key := deriveAgentKey(ctx)

	_, ok := p.registry.startModel(key, modelName)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureModelStateConflict)
	}
}

// afterModel handles both partial and terminal model responses. For
// partial responses, it updates cumulative usage state only. For
// terminal responses, it emits duration, usage, and increments the
// parent agent inference count exactly once. Never mutates the
// generate_content span.
func (p *Plugin) afterModel(ctx callbackContext, usage *genai.GenerateContentResponseUsageMetadata, partial bool, llmResponseError error) {
	key := deriveAgentKey(ctx)
	st := p.registry.getModel(key)
	if st == nil {
		return
	}

	// Update cumulative usage from the response.
	if usage != nil {
		st.usage = mapUsageMetadata(usage)
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
	otelCtx := context.Background()

	p.instr.RecordInferenceDuration(otelCtx, duration, p.system, termSt.model, otelgenai.Operation("generate_content"))
	p.instr.RecordInferenceUsage(otelCtx, termSt.usage, p.system, termSt.model, otelgenai.Operation("generate_content"))
	p.registry.incrementAgentInference(key)
}

// onModelError is observational only. It does not terminalize, emit,
// mutate, or retain an authoritative error. The terminal AfterModel
// error remains authoritative.
func (p *Plugin) onModelError() {
	// No-op: observational only.
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
