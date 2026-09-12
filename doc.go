// Package otelgenai provides framework-neutral OpenTelemetry GenAI
// instrumentation for Go applications.
//
// The library emits specification-pinned OpenTelemetry traces and metrics
// for generative AI operations including inference, streaming,
// embeddings, agent invocation, and tool execution. It is safe by
// default: no prompts, responses, tool arguments, tool results,
// credentials, cookies, or raw bodies are captured unless an explicit
// [ContentProjector] is configured.
//
// # Quick start
//
// Create an [Instrumenter] and use it to wrap provider SDK calls:
//
//	instr, err := otelgenai.New()
//	if err != nil { ... }
//
//	ctx, op := instr.StartInference(ctx, otelgenai.Request{
//	    Operation: otelgenai.Operation(otelgenai.OperationChat),
//	    Provider: "openai",
//	    Model:    "gpt-4o",
//	})
//	resp, err := client.Chat.Completions.New(ctx, params)
//	op.End(otelgenai.Response{
//	    Model:    resp.Model,
//	    ID:       resp.ID,
//	    Usage:    otelgenai.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens},
//	}, err)
//
// # Safety
//
// Telemetry is metadata-only by default. Content projection is opt-in
// via [WithContentProjector]. Projected content is validated against
// the convention schema and a byte limit. On any projector failure,
// panic, invalid shape, or oversize result, the projection is dropped
// with no raw fallback.
//
// # Semantic conventions
//
// The library pins a specific GenAI semantic-convention snapshot. All
// attribute keys, span names, instrument names, units, and boundaries
// are centralized in the internal semconv package. Convention upgrades
// are deliberate, reviewed changes.
package otelgenai
