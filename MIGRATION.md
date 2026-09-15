# Migration Guide

This document describes how to migrate between versions of otelgenai-go.

## Migrating to v0.6

v0.6 adds correlated OTel Logs API events, conversation correlation,
projected content, structured messages, and ADK event integration. It
also hardens release engineering and removes production adapter
dependencies on core internals.

### New features

- **Correlated events** via the OTel Logs API:
  - `WithLoggerProvider` option for explicit logger provider.
  - `WithConversationID` / `ConversationIDFromContext` for conversation
    correlation on spans and events (never on metrics).
  - `ProjectedContent` opaque bounded projection via
    `Instrumenter.ProjectContent`.
  - `EmitInferenceDetails`, `EmitToolDetails`, `EmitAgentOccurrence`
    for standard and repository-owned events.
  - `Message.Parts` / `MessagePart` / `SystemInstructionParts`
    structured canonical content.
- **Well-known constants**: `OperationChat`, `OperationGenerateContent`,
  `OperationTextCompletion`, `OperationEmbeddings`,
  `OperationInvokeAgent`, `OperationExecuteTool`, `SystemOpenAI`,
  `SystemAnthropic` exported from the core package.
- **Semantic span helpers**: `ApplySpanOutcome(ctx, err)` and
  `AugmentToolSpan(ctx, ToolSpanAttributes)` for framework adapters
  that need to decorate framework-owned spans without raw span
  mutation.
- **Version constant**: `otelgenai.Version` (`"0.6.0"`) used as the
  default instrumentation scope version.
- **InternalOperation conversation ID**: `gen_ai.conversation.id` is
  now attached to internal operation spans when present in context.

### Changed behavior

- All core operation `End` methods now use the panic-isolated
  `ClassifyError`. A panicking classifier no longer crashes `End`; it
  reports a `classifier.panic` diagnostic and sets `error.type=unknown`.
- The default instrumentation scope version is now `0.6.0` (was
  `0.1.0`). Override with `WithInstrumentationVersion` if needed.
- All adapter `go.mod` files now require core `v0.6.0`.
- All production adapter code no longer imports `internal/semconv` or
  `internal/safety`. Test-only imports remain.

### Privacy

All v0.6 additions preserve the safe-by-default contract:
- No prompts, responses, tool arguments, or results without a
  configured `ContentProjector`.
- No credentials, cookies, or raw bodies.
- No raw error messages in telemetry.
- Conversation IDs never appear on metrics.
- Event names are static; no dynamic values in event names.

## Migrating to v0.5

v0.5 adds the Google ADK Go framework integration and exported
adapter-facing core APIs while preserving backward compatibility with
v0.4.

### New features

- **ADK adapter module**: new `instrumentation/adk` module for Google
  ADK Go v2.3.0.
  - Install with `go get github.com/hmmftg/otelgenai-go/instrumentation/adk`.
  - Construct with `adkadapter.New(instr)`.
  - Register the plugin first in your ADK runner for strongest
    terminal-metric completeness.
  - See [ADK README](instrumentation/adk/README.md) for details.
- **Core adapter-facing APIs**: new exported methods on `Instrumenter`
  for framework integrations.
  - `ClassifyError(err error) ErrorType` — panic-isolated error
    classification; `ClassifyError(nil)` returns `ErrorTypeNone`.
  - `ReportInstrumentationFailure(f InstrumentationFailure)` — bounded
    low-cardinality diagnostic reporting.
  - `RecordInferenceUsage`, `RecordInferenceDuration`,
    `RecordAgentDuration`, `RecordAgentInferenceCalls`,
    `RecordAgentToolCalls`, `RecordToolDuration` — semantic-argument
    metric recording (not arbitrary `attribute.KeyValue`).
  - Existing metric semantics preserved: zero agent counts recorded
    once, only positive token components produce measurements.

### Compatibility

- No breaking changes to existing APIs.
- The ADK module is a separate Go module; it does not affect existing
  provider adapters.
- The ADK module is pinned to `google.golang.org/adk/v2 v2.3.0`;
  upgrading ADK must be treated as a deliberate compatibility change.

## Migrating to v0.4

v0.4 adds optional pricing/cost estimation and telemetry quality
helpers while preserving backward compatibility with v0.3.

### New features

- **Pricing subpackage**: new `pricing` package for estimated cost
  derivation from token usage.
  - `WithPricingResolver` option on `Instrumenter`.
  - `gen_ai.usage.estimated_cost` span attribute (repository-defined
    extension, not part of the pinned upstream semantic-convention
    contract).
  - `StaticResolver` for simple price-map-based resolution.
  - Custom `PricingResolver` implementations for alias resolution,
    regional pricing, etc.
- **testutil cardinality helpers**: `AssertLowCardinalityModel`,
  `AssertNoDynamicSpanNames`, `AssertNoRawErrors`,
  `AssertNoResourceURIsInSpanNames`, `AssertNoContentInAttributes`.
- **testutil cost assertions**: `AssertEstimatedCost`,
  `AssertNoEstimatedCost`.
- **Error contract**: documented cross-adapter error classification
  contract in `docs/error_contract.md`.

### Using the pricing resolver

```go
import (
    "github.com/hmmftg/otelgenai-go"
    "github.com/hmmftg/otelgenai-go/pricing"
)

resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
    {System: "openai", Model: "gpt-4o"}: {
        InputPerToken:  0.00001,
        OutputPerToken: 0.00003,
    },
})

instr, err := otelgenai.New(
    otelgenai.WithPricingResolver(resolver),
)
```

### Behavior notes

- Pricing is opt-in. If no resolver is configured, no cost telemetry is
  emitted; only a nil check remains.
- Cost calculation never fails the inference operation. All pricing
  failures (unknown model, invalid price, inconsistent usage, resolver
  panic) result in no cost attribute being emitted.
- The `Usage` Godoc was updated from "provider-reported billable usage"
  to "provider-reported token usage. The library does not infer missing
  usage values."

## Migrating to v0.3

v0.3 adds MCP instrumentation and core hardening while preserving
backward compatibility with v0.2.

### New features

- **MCP adapter**: new `instrumentation/mcp` module for Model Context
  Protocol instrumentation using `modelcontextprotocol/go-sdk` v1.7.0.
  - Client middleware: `ClientMiddleware(instr)` for `AddSendingMiddleware`.
  - Server middleware: `ServerMiddleware(instr)` for `AddReceivingMiddleware`.
  - Instruments `tools/call`, `resources/read`, `prompts/get` as
    INTERNAL GenAI operations.
  - W3C trace-context propagation via MCP `_meta`.
- **InternalOperation**: `StartInternalOperation` for generic INTERNAL
  spans with configured error classification.
- **ToolRequest.Attrs**: backward-compatible field for adapter-supplied
  attributes.

### Using the MCP adapter

```go
import (
    mcp "github.com/modelcontextprotocol/go-sdk/mcp"
    mcpadapter "github.com/hmmftg/otelgenai-go/instrumentation/mcp"
)

// Client-side.
client := mcp.NewClient(&mcp.Implementation{Name: "my-app", Version: "1.0"}, nil)
client.AddSendingMiddleware(mcpadapter.ClientMiddleware(instr))

// Server-side.
server := mcp.NewServer(&mcp.Implementation{Name: "my-server", Version: "1.0"}, nil)
server.AddReceivingMiddleware(mcpadapter.ServerMiddleware(instr))
```

### Semantic scope

v0.3 provides repository-level INTERNAL operation mapping for MCP. It
does not claim canonical MCP `CLIENT`/`SERVER` RPC span
instrumentation. Canonical MCP RPC span semantics are deferred until
upstream conventions stabilize (protocol 2026-07-28).

### Backward compatibility

All v0.2 APIs remain unchanged. The new `InternalOperation` type and
`ToolRequest.Attrs` field are additive.

## Migrating to v0.2

v0.2 adds new providers and public APIs while preserving backward
compatibility with v0.1.

### New features

- **Generic trace helpers**: `TraceInference` and `TraceAgent` provide
  provider-neutral wrappers for inference and agent operations.
- **Public test utilities**: the `testutil` package provides a recorder
  and semantic assertions for testing instrumentation.
- **OpenAI-compatible providers**: `WithSystem` option for OpenAI
  adapters to support Ollama, vLLM, Groq, and similar providers.
- **Google GenAI adapter**: new `instrumentation/google-genai` module
  for Google Gemini API and Vertex AI.

### Backward compatibility

All v0.1 APIs remain unchanged. Existing code using `StartInference`,
`StartAgent`, `StartTool`, `TraceTool`, and the OpenAI/Anthropic
adapters continues to work without modification.

### Using TraceInference

The new `TraceInference` helper wraps a callback with instrumentation:

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

### Using TraceAgent

```go
result, err := otelgenai.TraceAgent(ctx, instr, otelgenai.AgentRequest{
    Name: "my-agent",
}, func(ctx context.Context) (string, error) {
    // Your agent logic here.
    return "result", nil
})
```

### Using WithSystem for OpenAI-compatible providers

```go
client := oai.NewClient(option.WithBaseURL("http://localhost:11434/v1"))
chat := openai.NewChatCompletions(&client, instr, openai.WithSystem("ollama"))
```

### Using the Google GenAI adapter

```go
client, _ := genai.NewClient(ctx, &genai.ClientConfig{
    APIKey:  "your-api-key",
    Backend: genai.BackendGeminiAPI,
})
models := googlegenai.NewModels(client, instr)
resp, err := models.GenerateContent(ctx, "gemini-2.5-flash", contents, nil)
```

### Using testutil

```go
rec := testutil.NewRecorder()
defer rec.Shutdown(ctx)

instr, _ := otelgenai.New(
    otelgenai.WithTracerProvider(rec.TracerProvider()),
    otelgenai.WithMeterProvider(rec.MeterProvider()),
)

// ... perform instrumented operations ...

spans := rec.Spans()
testutil.AssertSpanName(t, spans[0], "chat gpt-4o")
testutil.AssertTokenUsage(t, rec.Collect(ctx), "gpt-4o", 100, 50)
```

### Internal conformance test package removed

The `internal/conformancetest` package has been replaced by the public
`testutil` package. Tests that imported `internal/conformancetest` should
migrate to `testutil`.

## v0.1 to v0.2 mapping

| v0.1 | v0.2 |
|------|------|
| `internal/conformancetest.AssertSpanName` | `testutil.AssertSpanName` |
| `internal/conformancetest.AssertAttr` | `testutil.AssertAttr` |
| `internal/conformancetest.AssertNoSentinel` | `testutil.AssertNoSentinel` |
| `internal/conformancetest.AssertAttrNotPresent` | `testutil.AssertAttrNotPresent` |
| `internal/conformancetest.AssertSpanStatus` | `testutil.AssertSpanStatus` |
