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
| Cache read tokens | `gen_ai.usage.cache_read_input_tokens` |
| Cache write tokens | `gen_ai.usage.cache_write_input_tokens` |
| Reasoning tokens | `gen_ai.usage.reasoning_tokens` |

### Cost attributes (repository-defined extension)

| Attribute | Key | Description |
|-----------|-----|-------------|
| Estimated cost | `gen_ai.usage.estimated_cost` | Estimated cost in USD, derived from token usage via a configured `PricingResolver`. |

This attribute is a **repository-defined extension** and is not part of
the pinned upstream semantic-convention contract. Upstream cost
conventions are still under active development; v0.4 therefore uses a
repository-defined span attribute rather than adopting an unstable
upstream cost convention. It does not introduce metric cardinality
because it is recorded only on spans. See [docs/pricing.md](pricing.md)
for details.

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

### MCP attributes (adapter-local)

These attributes are emitted by the MCP adapter (`instrumentation/mcp`)
and are intentionally kept adapter-local because upstream MCP semantic
conventions are still evolving.

| Attribute | Key | Methods |
|-----------|-----|---------|
| MCP method name | `mcp.method.name` | `tools/call`, `resources/read`, `prompts/get` |
| Resource URI | `mcp.resource.uri` | `resources/read` |
| Prompt name | `gen_ai.prompt.name` | `prompts/get` |

### Internal operation attributes

`InternalOperation` spans are generic INTERNAL spans with no
provider-specific or MCP-specific knowledge. They carry only the
supplied attributes plus `error.type` on failure. When a conversation
ID is present in the context (via `WithConversationID`), it is
attached as `gen_ai.conversation.id`.

### Correlation attributes

| Attribute | Key | Scope | Description |
|-----------|-----|-------|-------------|
| Conversation ID | `gen_ai.conversation.id` | traces, events | Canonical conversation/session identifier. Never added to metrics. |

The conversation ID is attached to spans via `WithConversationID` and
read from context by `StartInference`, `StartAgent`, `StartTool`, and
`StartInternalOperation`. It is also carried on correlated events. It
is never added to metrics to avoid high-cardinality time series.

`WithConversationID(ctx, "")` explicitly clears/shadows an inherited
ID: `ConversationIDFromContext` then returns `("", true)`, and spans
and events created from that context carry no `gen_ai.conversation.id`
attribute. The two-result lookup lets callers distinguish "absent"
from "explicitly set" (including an explicit clear).

### Event-only attributes

Correlated events (OTel Logs API) use some attribute names that differ
from the pinned span conventions. This divergence follows the current
upstream GenAI event schema and is documented as a v1.0 decision (see
Finding F9 in `docs/technical-review.md`).

| Attribute | Key | Notes |
|-----------|-----|-------|
| Provider name | `gen_ai.provider.name` | Event-only; spans use `gen_ai.system`. |
| Request stream | `gen_ai.request.stream` | Event-only; spans use `gen_ai.request.streaming`. |
| Cache read tokens | `gen_ai.usage.cache_read.input_tokens` | Event-only dotted form. |
| Cache write tokens | `gen_ai.usage.cache_write.input_tokens` | Event-only dotted form. |
| Reasoning tokens | `gen_ai.usage.reasoning.output_tokens` | Event-only dotted form. |

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

### MCP operations (repository semantic mapping)

v0.3 provides repository-level INTERNAL operation mapping for MCP.
It does not claim canonical MCP `CLIENT`/`SERVER` RPC span
instrumentation because upstream MCP semantic conventions are still
evolving (see
[open-telemetry/semantic-conventions-genai#437](https://github.com/open-telemetry/semantic-conventions-genai/issues/437)).

| MCP method | Core operation | Span name | Span kind | Agent counter |
|---|---|---|---|---|
| `tools/call` | `StartTool` | `execute_tool {name}` | INTERNAL | client +1, server +0 |
| `resources/read` | `StartInternalOperation` | `resources/read` | INTERNAL | 0 |
| `prompts/get` | `StartInternalOperation` | `prompts/get` | INTERNAL | 0 |

This is a repository semantic mapping, not canonical MCP RPC span
mapping. Canonical MCP `CLIENT`/`SERVER` RPC spans, session lifecycle
modeling, and duplicate-span suppression are deferred to a later
version.

## Span naming

Spans are named `{operation} {model}` for inference operations and
`{operation} {name}` for agent and tool operations. Span naming is delegated
to the core `StartInference`, `StartAgent`, and `StartTool` functions.

For MCP `resources/read` and `prompts/get`, the span name is the MCP
method name only (no URI or prompt name in the span name). Resource
URIs and prompt names are carried as attributes (`mcp.resource.uri`
and `gen_ai.prompt.name` respectively).

## Event names

Correlated events use static, event-specific names. Dynamic identity
belongs in attributes, never in event names.

| Event name | Scope | Description |
|------------|-------|-------------|
| `gen_ai.client.inference.operation.details` | Upstream standard | Opt-in inference operation details. |
| `otelgenai.execute_tool.operation.details` | Repository-owned | Tool execution details. |
| `otelgenai.agent.model.error_observed` | Repository-owned | Agent observed a model error. |
| `otelgenai.agent.tool.error_observed` | Repository-owned | Agent observed a tool error. |
| `otelgenai.agent.continued_after_error` | Repository-owned | Agent continued after a prior error. |

Repository-owned events use the `otelgenai.*` namespace to distinguish
them from upstream-standard events. These names are stable within v0.x.

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
