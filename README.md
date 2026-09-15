# otelgenai-go

Framework-neutral OpenTelemetry GenAI instrumentation for Go.

`otelgenai-go` emits specification-pinned OpenTelemetry traces and
metrics for generative AI operations. It wraps the official OpenAI,
Anthropic, and Google GenAI Go SDKs with typed service adapters that
create one logical span per provider operation, enclosing all automatic
SDK retries. It also instruments MCP (Model Context Protocol) client
and server operations.

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
- Not a RAG, memory, or workflow library.

## Architecture

```
otelgenai-go/
├── go.mod                         # Core module (framework-neutral)
├── instrumentation/openai/        # OpenAI adapter module
├── instrumentation/anthropic/     # Anthropic adapter module
├── instrumentation/google-genai/  # Google GenAI adapter module
├── instrumentation/mcp/           # MCP adapter module
└── instrumentation/adk/           # Google ADK Go adapter module
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

### OpenAI-compatible providers

For Ollama, vLLM, Groq, and other OpenAI-compatible providers, use the
official OpenAI SDK with a custom base URL and `WithSystem`:

```go
client := openai.NewClient(
    option.WithBaseURL("http://localhost:11434/v1"),
    option.WithAPIKey("dummy"),
)
chat := openaiadapter.NewChatCompletions(&client, instr, openaiadapter.WithSystem("ollama"))
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

### Google GenAI adapter

```go
instr, _ := otelgenai.New()
client, _ := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey:  "your-api-key",
    Backend: genai.BackendGeminiAPI,
})
models := googlegenai.NewModels(client, instr)

resp, err := models.GenerateContent(ctx, "gemini-2.5-flash", []*genai.Content{
    {Role: "user", Parts: []*genai.Part{{Text: "Hello!"}}},
}, nil)
```

For Vertex AI, set `Backend: genai.BackendVertexAI` with `Project` and
`Location`. The adapter automatically attributes telemetry to
`gen_ai.system=gcp.vertex_ai` (or `gcp.gemini` for the Gemini API
backend).

### MCP adapter

The MCP adapter instruments `tools/call`, `resources/read`, and
`prompts/get` as repository-level INTERNAL GenAI operations. It uses
the official `github.com/modelcontextprotocol/go-sdk` and propagates
W3C trace context via MCP `_meta`.

```go
instr, _ := otelgenai.New()

// Client-side instrumentation.
client := mcp.NewClient(&mcp.Implementation{Name: "my-client", Version: "1.0"}, nil)
client.AddSendingMiddleware(mcpadapter.ClientMiddleware(instr))

// Server-side instrumentation.
server := mcp.NewServer(&mcp.Implementation{Name: "my-server", Version: "1.0"}, nil)
server.AddReceivingMiddleware(mcpadapter.ServerMiddleware(instr))
```

v0.3 provides repository-level INTERNAL operation mapping. It does not
claim canonical MCP `CLIENT`/`SERVER` RPC span instrumentation; upstream
MCP semantic conventions are still evolving. When both client and
server middleware are enabled in the same process, duplicate spans are
expected and intentional (connected via `_meta` trace-context
propagation).

### Generic trace helpers

```go
resp, err := otelgenai.TraceInference(ctx, instr, otelgenai.Request{
    Operation: otelgenai.Operation("chat"),
    Provider:  "openai",
    Model:     "gpt-4o",
}, func(ctx context.Context) (otelgenai.Response, error) {
    // Your inference call here.
    return otelgenai.Response{Model: "gpt-4o"}, nil
})
```

### Estimated cost (v0.4)

Pricing is opt-in. Configure a `PricingResolver` to emit
`gen_ai.usage.estimated_cost` as a span attribute:

```go
import "github.com/hmmftg/otelgenai-go/pricing"

resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
    {System: "openai", Model: "gpt-4o"}: {
        InputPerToken:  0.00001,
        OutputPerToken: 0.00003,
    },
})

instr, err := otelgenai.New(otelgenai.WithPricingResolver(resolver))
```

Cost is estimated, not authoritative billing. All pricing failures
(unknown model, invalid price, inconsistent usage, resolver panic)
result in no cost attribute; the inference operation is never affected.
See [docs/pricing.md](docs/pricing.md) for the full token accounting
model and API.

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

### Events (opt-in, correlated logs)

Events are emitted through the OTel Logs API and correlated to the
active trace context. They are opt-in: no events without a logger
provider that accepts records, and no content without a configured
`ContentProjector`.

| Event name                                    | Content |
|-----------------------------------------------|---------|
| `gen_ai.client.inference.operation.details`   | Upstream standard: models, usage, finish reasons, projected system instructions / input / output messages |
| `otelgenai.execute_tool.operation.details`    | Repository-owned: projected tool arguments/result, error type |
| `otelgenai.agent.model.error_observed`        | Repository-owned: model error was observed |
| `otelgenai.agent.tool.error_observed`         | Repository-owned: tool error was observed |
| `otelgenai.agent.continued_after_error`       | Repository-owned: agent continued after an observed error (no retry/recovery claim) |

`WithConversationID(ctx, id)` attaches `gen_ai.conversation.id` to spans
and events; it is never added to metrics. Event timestamps are
occurrence times, not export times.

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
| `otelgenai-go/testutil`         | 1.27   | v1.46.0 | —                     |
| `instrumentation/openai`        | 1.27   | v1.46.0 | openai-go v1.12.0     |
| `instrumentation/anthropic`     | 1.27   | v1.46.0 | anthropic-sdk-go v1.72.0 |
| `instrumentation/google-genai`  | 1.27   | v1.46.0 | google.golang.org/genai v1.71.0 |
| `instrumentation/mcp`           | 1.27   | v1.46.0 | modelcontextprotocol/go-sdk v1.7.0 |
| `instrumentation/adk`           | 1.27   | v1.46.0 | google.golang.org/adk/v2 v2.3.0 |

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
