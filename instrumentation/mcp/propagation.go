package mcp

import (
	"context"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// metaCarrier implements propagation.TextMapCarrier over mcp.Meta.
// It provides read/write access to the trace-context keys
// (traceparent, tracestate, baggage) in the MCP _meta map.
type metaCarrier struct {
	meta map[string]any
}

func (c metaCarrier) Get(key string) string {
	if c.meta == nil {
		return ""
	}
	v, ok := c.meta[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func (c metaCarrier) Set(key string, value string) {
	if c.meta == nil {
		c.meta = make(map[string]any)
	}
	c.meta[key] = value
}

func (c metaCarrier) Keys() []string {
	if c.meta == nil {
		return nil
	}
	keys := make([]string, 0, len(c.meta))
	for k := range c.meta {
		keys = append(keys, k)
	}
	return keys
}

// InjectTraceContext injects W3C trace context from ctx into a cloned
// mcp.Meta map using the application's configured global TextMapPropagator.
// The input meta is never mutated; a cloned map is returned.
//
// If the configured global propagator is a no-op, the function still
// returns a cloned metadata map and never mutates the input. Ownership
// semantics are preserved regardless of propagation configuration.
//
// Existing _meta values are preserved; only trace-context keys
// (traceparent, tracestate, baggage) are set.
func InjectTraceContext(ctx context.Context, meta mcp.Meta) mcp.Meta {
	// Clone the input map; never mutate the caller's map.
	cloned := make(mcp.Meta)
	for k, v := range meta {
		cloned[k] = v
	}
	carrier := metaCarrier{meta: cloned}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return cloned
}

// ExtractTraceContext extracts W3C trace context from mcp.Meta using
// the application's configured global TextMapPropagator, returning a
// derived context with the remote parent span.
//
// If meta is nil or empty, the original ctx is returned unchanged.
func ExtractTraceContext(ctx context.Context, meta mcp.Meta) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	carrier := metaCarrier{meta: meta}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// Ensure propagation.TextMapPropagator is used (compile-time check).
var _ propagation.TextMapPropagator = otel.GetTextMapPropagator()
