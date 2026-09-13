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
// The server middleware derives the operation context from the incoming
// SDK request context, preserving cancellation, deadlines, and other
// request-scoped values. It replaces only the trace parent with the
// remote trace context extracted from _meta, and strips the local
// agentObserver so that server-side tools/call does not increment any
// local agent's tool-call counter.
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

			// Extract remote trace context from _meta and apply it to
			// the incoming context. This preserves cancellation, deadlines,
			// and other request-scoped values from the SDK while replacing
			// only the trace parent.
			var meta mcp.Meta
			if req != nil && req.GetParams() != nil {
				meta = mcp.Meta(req.GetParams().GetMeta())
			}
			ctx = ExtractTraceContext(ctx, meta)

			// Strip the local agentObserver so server-side operations
			// do not increment any local agent's counters. The remote
			// trace context above is the only telemetry state that
			// crosses the protocol boundary.
			ctx = otelgenai.WithoutAgentObserver(ctx)

			target := extractTarget(method, req)
			ctx, op := startOperation(ctx, instr, method, target)

			result, err := next(ctx, method, req)
			op.End(err)
			return result, err
		}
	}
}
