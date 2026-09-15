package otelgenai

import "context"

type conversationIDKey struct{}

// WithConversationID returns a context carrying the canonical
// conversation identifier. It is attached to spans and correlated
// events as gen_ai.conversation.id. It is never added to metrics.
//
// Callers should pass an existing framework or application
// conversation/session identifier. The library never generates UUIDs,
// trace IDs, or content hashes as fallbacks.
func WithConversationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, conversationIDKey{}, id)
}

// ConversationIDFromContext returns the conversation identifier
// attached by WithConversationID, or "".
func ConversationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(conversationIDKey{}).(string)
	return id
}
