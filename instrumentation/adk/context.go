package adk

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// callbackContext is the minimal interface the adapter needs from ADK
// callback contexts. ADK's callbackContextWrapper supports InvocationID,
// Branch, and AgentName but not Path, RunID, or FunctionCallID.
type callbackContext interface {
	context.Context
	InvocationID() string
	Branch() string
	AgentName() string
}

// deriveAgentKey extracts the adapter identity from a callback context.
func deriveAgentKey(ctx callbackContext) agentStateKey {
	return agentStateFrom(ctx.InvocationID(), ctx.Branch(), ctx.AgentName())
}

// agentStateFrom constructs an agentStateKey from its components.
func agentStateFrom(invocationID, branch, agentName string) agentStateKey {
	return agentStateKey{
		InvocationID: invocationID,
		Branch:       branch,
		AgentName:     agentName,
	}
}

// activeSpanKey extracts the composite {TraceID, SpanID} from the active
// span in the context. Returns the key and true if both components are
// valid; returns false otherwise.
func activeSpanKey(ctx context.Context) (toolStateKey, bool) {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return toolStateKey{}, false
	}
	return toolStateKey{
		TraceID: sc.TraceID(),
		SpanID:  sc.SpanID(),
	}, true
}
