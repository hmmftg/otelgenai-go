package adk

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// toolNameProvider is the minimal interface the adapter needs from an
// ADK tool to resolve the tool name.
type toolNameProvider interface {
	Name() string
}

// beforeTool creates tool state keyed by the composite {TraceID, SpanID}
// of the active execute_tool span. Requires a valid span context;
// invalid spans fail closed. Sets gen_ai.tool.type unconditionally and
// resolves gen_ai.system through the configured resolver. When content
// events are enabled, projected tool call arguments are stored for the
// terminal tool-details event. A pending child error on the agent
// produces a continued_after_error occurrence.
func (p *Plugin) beforeTool(ctx context.Context, t toolNameProvider, args map[string]any, occurredAt time.Time) {
	agentKey := agentStateKeyFromContext(ctx)

	if et, ok := p.registry.takePendingError(agentKey); ok {
		p.instr.EmitAgentOccurrence(ctx, otelgenai.AgentOccurrence{
			Kind:           otelgenai.OccurrenceContinuedAfterError,
			AgentName:      agentKey.AgentName,
			Operation:      otelgenai.Operation("execute_tool"),
			ConversationID: sessionIDFromContext(ctx),
			ErrorType:      et,
			OccurredAt:     occurredAt,
		})
	}

	key, ok := activeSpanKey(ctx)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return
	}

	invocationID := invocationIDFromContext(ctx)
	name := ""
	if t != nil {
		name = t.Name()
	}
	st, ok := p.registry.startTool(key, agentKey, invocationID, name)
	if !ok || st == nil {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return
	}

	if p.instr.ContentEventsEnabled(ctx) {
		p.registry.setToolArguments(key, p.projectToolArguments(args))
	}

	// Set gen_ai.tool.type = function unconditionally (adapter-owned).
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(semconv.AttrGenAIToolType, semconv.ToolTypeFunction))

	// Resolve gen_ai.system through the configured resolver.
	if p.toolSystemResolver != nil {
		var system string
		panicErr := safety.GuardedCall(func() error {
			system = p.toolSystemResolver(t)
			return nil
		})
		if panicErr != nil {
			p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureToolSystemResolverPanic)
		} else if system != "" {
			span.SetAttributes(attribute.String(semconv.AttrGenAISystem, system))
		}
	}
}

// afterTool emits tool duration, the tool-details event, increments the
// captured parent agent tool count, classifies the final error, and
// augments the active execute_tool span with error.type and status.
// The details event is emitted while the span is still active so it is
// correlated to the tool span context. Returns (nil, nil) to indicate
// no replacement.
func (p *Plugin) afterTool(ctx context.Context, result map[string]any, err error, occurredAt time.Time) {
	key, ok := activeSpanKey(ctx)
	if !ok {
		return
	}

	st := p.registry.endTool(key)
	if st == nil {
		return
	}

	duration := time.Since(st.start)

	p.instr.RecordToolDuration(ctx, duration, st.name)
	p.registry.incrementAgentTool(st.agentKey)

	var et otelgenai.ErrorType
	if err != nil {
		et = p.instr.ClassifyError(err)
		if et == otelgenai.ErrorTypeNone {
			et = otelgenai.ErrorTypeUnknown
		}
		p.registry.markPendingError(st.agentKey, et)
	}

	// Emit the tool-details event while the span is still active. A
	// failed execution does not invent a result; only the projected
	// arguments and the error type are reported.
	details := otelgenai.ToolDetails{
		ToolName:       st.name,
		ConversationID: sessionIDFromContext(ctx),
		Arguments:      st.arguments,
		ErrorType:      et,
		OccurredAt:     occurredAt,
	}
	if err == nil && p.instr.ContentEventsEnabled(ctx) {
		details.Result = p.projectToolResult(result)
	}
	p.instr.EmitToolDetails(ctx, details)

	// Augment the active execute_tool span with error.type and status.
	span := trace.SpanFromContext(ctx)
	classifyAndSetError(span, p.instr, err)
}

// onToolError records the observed tool error as an occurrence event
// and marks the agent with a pending error so a subsequent
// BeforeModel/BeforeTool reports continued_after_error. It does not
// terminalize, delete, or mutate state; the AfterTool callback remains
// authoritative.
func (p *Plugin) onToolError(ctx context.Context, err error, occurredAt time.Time) {
	if err == nil {
		return
	}
	agentKey := agentStateKeyFromContext(ctx)
	et := p.instr.ClassifyError(err)
	if et == otelgenai.ErrorTypeNone {
		et = otelgenai.ErrorTypeUnknown
	}
	p.registry.markPendingError(agentKey, et)
	p.instr.EmitAgentOccurrence(ctx, otelgenai.AgentOccurrence{
		Kind:           otelgenai.OccurrenceToolErrorObserved,
		AgentName:      agentKey.AgentName,
		Operation:      otelgenai.Operation("execute_tool"),
		ConversationID: sessionIDFromContext(ctx),
		ErrorType:      et,
		OccurredAt:     occurredAt,
	})
}

// agentStateKeyFromContext extracts the agent state key from a context
// that implements the callbackContext interface. Returns a zero key
// if the context does not implement it.
func agentStateKeyFromContext(ctx context.Context) agentStateKey {
	if cbCtx, ok := ctx.(callbackContext); ok {
		return deriveAgentKey(cbCtx)
	}
	return agentStateKey{}
}

// invocationIDFromContext extracts the invocation ID from a context
// that implements the callbackContext interface. Returns empty string
// if the context does not implement it.
func invocationIDFromContext(ctx context.Context) string {
	if cbCtx, ok := ctx.(callbackContext); ok {
		return cbCtx.InvocationID()
	}
	return ""
}

// sessionIDFromContext extracts the session ID from a context that
// implements the callbackContext interface. Returns empty string if the
// context does not implement it.
func sessionIDFromContext(ctx context.Context) string {
	if cbCtx, ok := ctx.(callbackContext); ok {
		return cbCtx.SessionID()
	}
	return ""
}
