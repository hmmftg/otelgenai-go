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
// resolves gen_ai.system through the configured resolver. Returns
// (nil, nil) to indicate no replacement.
func (p *Plugin) beforeTool(ctx context.Context, t toolNameProvider) {
	key, ok := activeSpanKey(ctx)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return
	}

	agentKey := agentStateKeyFromContext(ctx)
	invocationID := invocationIDFromContext(ctx)
	name := ""
	if t != nil {
		name = t.Name()
	}
	_, ok = p.registry.startTool(key, agentKey, invocationID, name)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return
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

// afterTool emits tool duration, increments the captured parent agent
// tool count, classifies the final error, and augments the active
// execute_tool span with error.type and status. Returns (nil, nil) to
// indicate no replacement.
func (p *Plugin) afterTool(ctx context.Context, err error) {
	key, ok := activeSpanKey(ctx)
	if !ok {
		return
	}

	st := p.registry.endTool(key)
	if st == nil {
		return
	}

	duration := time.Since(st.start)
	otelCtx := context.Background()

	p.instr.RecordToolDuration(otelCtx, duration, st.name)
	p.registry.incrementAgentTool(st.agentKey)

	// Augment the active execute_tool span with error.type and status.
	span := trace.SpanFromContext(ctx)
	classifyAndSetError(span, p.instr, err)
}

// onToolError is observational only. It does not terminalize, emit,
// delete, or mutate. The AfterTool callback remains authoritative.
func (p *Plugin) onToolError() {
	// No-op: observational only.
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
