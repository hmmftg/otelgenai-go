package adk

import (
	"time"

	"google.golang.org/genai"
)

// beforeAgent creates agent state keyed by the callback-visible identity.
// It does not mutate the invoke_agent span. Returns (nil, nil) to
// indicate no replacement.
func (p *Plugin) beforeAgent(ctx callbackContext) (*genai.Content, error) {
	key := deriveAgentKey(ctx)
	p.registry.startAgent(key, ctx.AgentName())
	return nil, nil
}

// afterAgent emits agent metrics once and deletes agent state. It does
// not synthesize error.type from child errors and does not mutate the
// invoke_agent span. Returns (nil, nil) to indicate no replacement.
func (p *Plugin) afterAgent(ctx callbackContext) (*genai.Content, error) {
	key := deriveAgentKey(ctx)
	st := p.registry.endAgent(key)
	if st == nil {
		return nil, nil
	}

	duration := time.Since(st.start)

	p.instr.RecordAgentDuration(ctx, duration, st.name)
	p.instr.RecordAgentInferenceCalls(ctx, st.inferenceCalls, st.name)
	p.instr.RecordAgentToolCalls(ctx, st.toolCalls, st.name)

	return nil, nil
}

// afterRun performs invocation-scoped cleanup of abandoned lifecycle
// state. It is teardown-only: it does not emit synthetic terminal metrics
// and does not mutate any span. It removes only state belonging to the
// current invocation, preserving concurrent runs.
func (p *Plugin) afterRun(invocationID string) {
	p.registry.cleanupInvocation(invocationID)
}
