package mcp

import (
	"context"

	"github.com/hmmftg/otelgenai-go"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ClientMiddleware returns an MCP middleware that instruments outgoing
// client-side MCP operations (tools/call, resources/read, prompts/get)
// as INTERNAL GenAI operations. It also injects W3C trace context into a
// cloned params._meta for server-side propagation.
//
// The middleware starts the operation FIRST, then injects the
// span-enriched context into _meta. This ensures the server span's remote
// parent is the client span, not the caller's parent span.
//
// Non-instrumented methods pass through completely unchanged: no span
// creation, no context modification, no _meta injection.
//
// The adapter uses the application's configured global TextMapPropagator
// via otel.GetTextMapPropagator(). It does not globally mutate propagator
// configuration.
func ClientMiddleware(instr *otelgenai.Instrumenter) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if !isInstrumented(method) {
				return next(ctx, method, req)
			}

			target := extractTarget(method, req)
			ctx, op := startOperation(ctx, instr, method, target)

			// Inject the span-enriched context into a cloned _meta.
			// This must happen AFTER startOperation so the traceparent
			// contains the client span's SpanID.
			if req != nil && req.GetParams() != nil {
				params := req.GetParams()
				clonedMeta := InjectTraceContext(ctx, mcp.Meta(params.GetMeta()))
				params.SetMeta(clonedMeta)
			}

			result, err := next(ctx, method, req)
			op.End(err)
			return result, err
		}
	}
}
