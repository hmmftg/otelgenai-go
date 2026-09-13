# Migration Guide

This document describes how to migrate between versions of otelgenai-go.

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
