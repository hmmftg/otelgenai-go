package otelgenai

import "context"

// TraceInference wraps a GenAI inference call with instrumentation. It
// starts an inference operation, invokes fn with the derived context,
// ends the operation with the callback's canonical response and error,
// and returns those values unchanged to the caller. Telemetry is
// derived internally from those values via op.End(resp, err).
//
// Nil/no-op semantics follow the existing TraceTool contract:
//   - in is never expected to be nil; callers construct an *Instrumenter
//     via New. A nil in is a programming error and is not specially
//     handled (matching TraceTool, which would panic on in.StartTool).
//   - When instrumentation is disabled or required metadata is absent,
//     TraceInference still invokes the callback and returns its
//     (Response, error) unchanged, while producing no telemetry.
//   - If the callback returns an error, op.End classifies it via the
//     configured ErrorClassifier and records error.type + span status;
//     the raw error is returned to the caller unchanged.
//   - If the callback panics, the panic propagates to the caller.
//     TraceInference does not recover panics or manufacture synthetic
//     errors.
//
// Custom embedding callers use TraceInference with Request.Operation
// set to the embeddings operation; a separate TraceEmbedding helper is
// intentionally not provided in v0.2.
func TraceInference(
	ctx context.Context,
	in *Instrumenter,
	req Request,
	fn func(context.Context) (Response, error),
) (Response, error) {
	ctx, op := in.StartInference(ctx, req)
	resp, err := fn(ctx)
	op.End(resp, err)
	return resp, err
}

// TraceAgent wraps a local agent invocation with instrumentation. It
// starts an agent span, invokes fn with the child context, ends the
// span using the callback error, and preserves the callback's result
// and error.
//
// Nil/no-op semantics match TraceTool:
//   - in is never expected to be nil; a nil in is a programming error.
//   - When instrumentation is disabled, TraceAgent still invokes the
//     callback and returns its (T, error) unchanged, while producing no
//     telemetry.
//   - If the callback panics, the panic propagates to the caller.
func TraceAgent[T any](
	ctx context.Context,
	in *Instrumenter,
	req AgentRequest,
	fn func(context.Context) (T, error),
) (T, error) {
	ctx, op := in.StartAgent(ctx, req)
	result, err := fn(ctx)
	op.End(err)
	return result, err
}
