# Semantic Conventions

This document describes the OpenTelemetry semantic conventions emitted by
otelgenai-go. The conventions are pinned to a private snapshot of the GenAI
semantic conventions (development), compatible with semconv/v1.30.0 generation.

## Pinned convention version

The attribute keys, metric names, and span naming templates live in
`internal/semconv/semconv.go` and are intentionally private. They are not part
of the public API and may change between minor versions until v1.0 stabilizes
the convention contract.

## Span attributes

### Common attributes

| Attribute | Key | Description |
|-----------|-----|-------------|
| GenAI system | `gen_ai.system` | The GenAI provider or backend. |
| Operation name | `gen_ai.operation.name` | The operation being performed. |
| Request model | `gen_ai.request.model` | The model requested by the caller. |
| Response model | `gen_ai.response.model` | The model version reported by the provider. |
| Response ID | `gen_ai.response.id` | The provider response identifier. |
| Request streaming | `gen_ai.request.streaming` | Whether the request is streaming. |

### Request configuration attributes

| Attribute | Key |
|-----------|-----|
| Max tokens | `gen_ai.request.max_tokens` |
| Temperature | `gen_ai.request.temperature` |
| Top P | `gen_ai.request.top_p` |
| Seed | `gen_ai.request.seed` |
| Stop sequences | `gen_ai.request.stop_sequences` |

### Usage attributes

| Attribute | Key |
|-----------|-----|
| Input tokens | `gen_ai.usage.input_tokens` |
| Output tokens | `gen_ai.usage.output_tokens` |
| Cache read tokens | `gen_ai.usage.cache_read_tokens` |
| Reasoning tokens | `gen_ai.usage.reasoning_tokens` |

### Response attributes

| Attribute | Key |
|-----------|-----|
| Finish reasons | `gen_ai.response.finish_reasons` |
| Time to first chunk | `gen_ai.response.time_to_first_chunk` |

### Error attributes

| Attribute | Key |
|-----------|-----|
| Error type | `error.type` |

### Agent attributes

| Attribute | Key |
|-----------|-----|
| Agent name | `gen_ai.agent.name` |

### Tool attributes

| Attribute | Key |
|-----------|-----|
| Tool name | `gen_ai.tool.name` |
| Tool type | `gen_ai.tool.type` |
| Tool call ID | `gen_ai.tool.call.id` |

### Opt-in content attributes (disabled by default)

These attributes are only emitted when a `ContentProjector` is configured.
By default, no content is captured.

| Attribute | Key |
|-----------|-----|
| Input messages | `gen_ai.input.messages` |
| Output messages | `gen_ai.output.messages` |
| System instructions | `gen_ai.system_instructions` |
| Tool definitions | `gen_ai.tool.definitions` |
| Tool call arguments | `gen_ai.tool.call.arguments` |
| Tool call result | `gen_ai.tool.call.result` |

## Provider system values

| Provider | `gen_ai.system` value |
|----------|---------------------|
| OpenAI | `openai` |
| OpenAI-compatible (Ollama, vLLM, Groq, etc.) | configured via `WithSystem` |
| Anthropic | `anthropic` |
| Google GenAI - Gemini API backend | `gcp.gemini` |
| Google GenAI - Vertex AI backend | `gcp.vertex_ai` |
| Google GenAI - unknown/zero backend | `gcp.gen_ai` |

## Operation names

| Operation | Value |
|-----------|-------|
| Chat | `chat` |
| Generate content | `generate_content` |
| Text completion | `text_completion` |
| Embeddings | `embeddings` |
| Invoke agent | `invoke_agent` |
| Execute tool | `execute_tool` |

## Span naming

Spans are named `{operation} {model}` for inference operations and
`{operation} {name}` for agent and tool operations. Span naming is delegated
to the core `StartInference`, `StartAgent`, and `StartTool` functions.

## Metrics

| Metric | Unit | Description |
|--------|------|-------------|
| `gen_ai.client.operation.duration` | `s` | Duration of GenAI client operations. |
| `gen_ai.client.token.usage` | `{token}` | Number of tokens used by GenAI client operations. |
| `gen_ai.client.operation.time_to_first_chunk` | `s` | Time to first output chunk for streaming. |
| `gen_ai.client.operation.time_per_output_chunk` | `s` | Time between output chunks for streaming. |
| `gen_ai.invoke_agent.duration` | `s` | Duration of GenAI agent invocations. |
| `gen_ai.invoke_agent.inference_calls` | `{inference_call}` | Number of inference calls made by an agent. |
| `gen_ai.invoke_agent.tool_calls` | `{tool_call}` | Number of tool calls made by an agent. |
| `gen_ai.execute_tool.duration` | `s` | Duration of GenAI tool executions. |

## Safety boundary

By default, otelgenai-go is metadata-only:

- No prompt text
- No generated output text
- No raw request or response bodies
- No credentials, cookies, or secrets
- No raw error messages

Content capture requires an explicit `ContentProjector` configured via
`otelgenai.WithContentProjector`. Projections are bounded by
`WithProjectionLimit` (default 4096 bytes) and dropped on overflow with no
raw fallback.

## Versioning policy

The semantic conventions are private and pinned. They will remain private
until v1.0 stabilizes the convention contract. A public semantic-convention
package is explicitly out of scope for v0.x.
