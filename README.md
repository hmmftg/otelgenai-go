# otelgenai-go

Framework-neutral OpenTelemetry GenAI instrumentation for Go.

`otelgenai-go` emits specification-pinned OpenTelemetry traces and
metrics for generative AI operations. It wraps the official OpenAI and
Anthropic Go SDKs with typed service adapters that create one logical
span per provider operation, enclosing all automatic SDK retries.

## Why?

The Go AI ecosystem has many frameworks and clients, but portable
production observability for GenAI is underserved. Existing
instrumentation is often tied to a framework, provider, or vendor
backend. `otelgenai-go` fills this gap with a small, framework-neutral
core and isolated provider adapters.

## Non-goals

- Not another AI framework or agent orchestrator.
- Not an exporter or collector configuration library.
- Not a cost/pricing engine.
- Not a retry, routing, caching, or rate-limiting library.
- Not a RAG, memory, workflow, or MCP instrumentation library (yet).

## Architecture

```
otelgenai-go/
├── go.mod                      # Core module (framework-neutral)
├── instrumentation/openai/     # OpenAI adapter module
└── instrumentation/anthropic/  # Anthropic adapter module
```

The core module depends only on OpenTelemetry API packages. Each
adapter module depends on the core and its official provider SDK.
Provider SDK types never leak into the core's public API.

## Safe defaults

Telemetry is metadata-only by default:

- No prompts, responses, tool arguments, or tool results.
- No credentials, cookies, authorization headers, or raw HTTP bodies.
- No raw error messages (low-cardinality `error.type` only).
- No content capture unless an explicit `ContentProjector` is
  configured.

When a projector is configured, projected content is validated against
the convention schema and a byte limit. On any projector failure, panic,
invalid shape, or oversize result, the projection is dropped with no
raw fallback.

## Quick start

### Manual instrumentation

```go
instr, err := otelgenai.New()
if err != nil { ... }

ctx, op := instr.StartInference(ctx, otelgenai.Request{
    Operation: otelgenai.Operation("chat"),
    Provider:  "openai",
    Model:     "gpt-4o",
})
resp, err := client.Chat.Completions.New(ctx, params)
op.End(otelgenai.Response{
    Model:    resp.Model,
    ID:       resp.ID,
    Usage:    otelgenai.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens},
}, err)
```

### OpenAI adapter

```go
instr, _ := otelgenai.New()
client := openai.NewClient()
chat := openaiadapter.NewChatCompletions(&client, instr)

resp, err := chat.New(ctx, openai.ChatCompletionNewParams{
    Model:    openai.ChatModelGPT4o,
    Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello!")},
})
```

### Anthropic adapter

```go
instr, _ := otelgenai.New()
client := anthropic.NewClient()
msgs := anthropicadapter.NewMessages(&client, instr)

resp, err := msgs.New(ctx, anthropic.MessageNewParams{
    Model:     anthropic.ModelClaude3Haiku,
    MaxTokens: 1024,
    Messages:  []anthropic.MessageParamUnion{anthropic.NewUserMessage(anthropic.NewTextBlock("Hello!"))},
})
```

## Emitted telemetry

### Spans

| Span name                  | Kind     | Operation         |
|---------------------------|----------|-------------------|
| `{operation} {model}`     | CLIENT   | chat, generate_content, text_completion, embeddings |
| `invoke_agent {name}`     | INTERNAL | invoke_agent      |
| `execute_tool {name}`     | INTERNAL | execute_tool      |

### Metrics

| Instrument                              | Unit           | Description                          |
|----------------------------------------|----------------|--------------------------------------|
| `gen_ai.client.operation.duration`     | s              | Duration of GenAI client operations  |
| `gen_ai.client.token.usage`            | {token}        | Tokens used by GenAI operations      |
| `gen_ai.client.operation.time_to_first_chunk` | s      | Time to first output chunk           |
| `gen_ai.client.operation.time_per_output_chunk` | s     | Time between output chunks           |
| `gen_ai.invoke_agent.duration`         | s              | Duration of agent invocations         |
| `gen_ai.invoke_agent.inference_calls`   | {inference_call} | Inference calls per agent          |
| `gen_ai.invoke_agent.tool_calls`        | {tool_call}    | Tool calls per agent                 |
| `gen_ai.execute_tool.duration`          | s              | Duration of tool executions          |

## Pinned convention version

GenAI semantic conventions are still in development. This library
pins a specific snapshot compatible with the
`go.opentelemetry.io/otel/semconv/v1.30.0` generation. All attribute
keys, span names, instrument names, units, and boundaries are
centralized in `internal/semconv`. Convention upgrades are deliberate,
reviewed changes.

## Compatibility

| Module                          | Go     | OTel    | Provider SDK           |
|---------------------------------|--------|---------|-----------------------|
| `otelgenai-go` (core)           | 1.27   | v1.46.0 | —                     |
| `instrumentation/openai`        | 1.27   | v1.46.0 | openai-go v1.12.0     |
| `instrumentation/anthropic`     | 1.27   | v1.46.0 | anthropic-sdk-go v1.72.0 |

## Stream-close responsibility

Callers are responsible for closing abandoned streams. The library
finalizes telemetry on every observable terminal path (terminal event,
stream error, cancellation, EOF, Close), but an unreachable unclosed
stream cannot be finalized reliably.

## Comparison with existing Go instrumentation

| Feature                        | otelgenai-go | otelhttp | Provider SDKs |
|--------------------------------|-------------|----------|---------------|
| GenAI semantic conventions     | Yes         | No       | No            |
| One span per logical operation | Yes         | Per HTTP | Per attempt   |
| Safe-by-default content        | Yes         | N/A      | No            |
| Provider-independent core      | Yes         | Yes      | No            |
| Framework-neutral              | Yes         | Yes      | No            |

## License

MIT.
