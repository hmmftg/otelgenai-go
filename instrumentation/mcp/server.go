package mcp

import (
	"context"

	"github.com/hmmftg/otelgenai-go"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerMiddleware returns an MCP middleware that instruments incoming
// server-side MCP operations (tools/call, resources/read, prompts/get)
// as INTERNAL GenAI operations. It extracts W3C trace context from
// request params._meta to establish the remote parent span.
//
// The server middleware extracts only the remote trace context from
// _meta and creates a fresh context for the server operation. It does
// NOT propagate local agentObserver state across the protocol boundary.
// This ensures server-side tools/call does not increment any local
// agent's tool-call counter.
//
// Non-instrumented methods pass through completely unchanged: no span
// creation, no context modification, no _meta extraction.
//
// The adapter uses the application's configured global TextMapPropagator
// via otel.GetTextMapPropagator(). It does not globally mutate propagator
// configuration.
func ServerMiddleware(instr *otelgenai.Instrumenter) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if !isInstrumented(method) {
				return next(ctx, method, req)
			}

			// Extract remote trace context from _meta into a FRESH context.
			// Do NOT use the original ctx as base, because it may contain
			// local agentObserver state that must not propagate across the
			// protocol boundary. Only the remote trace context is carried.
			var meta mcp.Meta
			if req != nil && req.GetParams() != nil {
				meta = mcp.Meta(req.GetParams().GetMeta())
			}
			ctx = ExtractTraceContext(context.Background(), meta)

			target := extractTarget(method, req)
			ctx, op := startOperation(ctx, instr, method, target)

			result, err := next(ctx, method, req)
			op.End(err)
			return result, err
		}
	}
}
