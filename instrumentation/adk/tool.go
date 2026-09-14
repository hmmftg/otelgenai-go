package adk

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// beforeTool creates tool state keyed by the composite {TraceID, SpanID}
// of the active execute_tool span. Requires a valid span context;
// invalid spans fail closed. Sets gen_ai.tool.type unconditionally and
// resolves gen_ai.system through the configured resolver. Returns
// (nil, nil) to indicate no replacement.
func (p *Plugin) beforeTool(ctx agent.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
	key, ok := activeSpanKey(ctx)
	if !ok {
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return nil, nil
	}

	agentKey := deriveAgentKey(ctx)
	_, ok = p.registry.startTool(key, agentKey, ctx.InvocationID(), t.Name())
	if !ok {
		// Invalid span context; fail closed.
		p.instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
		return nil, nil
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

	return nil, nil
}

// afterTool emits tool duration, increments the captured parent agent
// tool count, classifies the final error, and augments the active
// execute_tool span with error.type and status. Returns (nil, nil) to
// indicate no replacement.
func (p *Plugin) afterTool(ctx agent.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
	key, ok := activeSpanKey(ctx)
	if !ok {
		return nil, nil
	}

	st := p.registry.endTool(key)
	if st == nil {
		return nil, nil
	}

	duration := time.Since(st.start)
	otelCtx := context.Background()

	p.instr.RecordToolDuration(otelCtx, duration, st.name)
	p.registry.incrementAgentTool(st.agentKey)

	// Augment the active execute_tool span with error.type and status.
	span := trace.SpanFromContext(ctx)
	classifyAndSetError(span, p.instr, err)

	return nil, nil
}

// onToolError is observational only. It does not terminalize, emit,
// delete, or mutate. The AfterTool callback remains authoritative.
// Returns (nil, nil) to indicate no replacement.
func (p *Plugin) onToolError(ctx agent.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
	return nil, nil
}
