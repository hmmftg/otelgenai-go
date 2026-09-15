package otelgenai

import "context"

type conversationIDKey struct{}

// WithConversationID returns a context carrying the canonical
// conversation identifier. It is attached to spans and correlated
// events as gen_ai.conversation.id. It is never added to metrics.
//
// A non-empty id sets (or shadows an inherited) conversation ID. An
// empty id explicitly clears/shadows any inherited value: the context
// entry is stored either way, so a child context cannot re-inherit a
// cleared ID.
//
// Callers should pass an existing framework or application
// conversation/session identifier. The library never generates UUIDs,
// trace IDs, or content hashes as fallbacks.
func WithConversationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, conversationIDKey{}, id)
}

// ConversationIDFromContext returns the conversation identifier
// attached by WithConversationID and reports whether one is present.
// The second result is false when no ID was attached (absent) and true
// when one is set — including an explicitly cleared empty ID.
//
// Telemetry omits gen_ai.conversation.id in both the absent and the
// explicitly cleared cases; the boolean lets callers distinguish the
// two states.
func ConversationIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(conversationIDKey{}).(string)
	return id, ok
}
