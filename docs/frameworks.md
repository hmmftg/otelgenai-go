# Framework Integrations

This document lists the framework integrations provided by otelgenai-go.

## Google ADK Go

Module: `github.com/hmmftg/otelgenai-go/instrumentation/adk`

Pinned to `google.golang.org/adk/v2 v2.3.0`.

Integrates through ADK's public plugin API. Reuses ADK-owned semantic spans
(`invoke_agent`, `generate_content`, `execute_tool`) without duplication.
Emits existing otelgenai metrics through constrained core APIs.

See [ADK integration guide](integrations/adk.md) and the
[ADK adapter README](../instrumentation/adk/README.md) for details.

### Key Design Decisions

- **Native span reuse**: the adapter creates no semantic spans.
- **Non-interception**: every intercept-capable callback returns `(nil, nil)`.
- **Composite tool identity**: tool state keyed by `{TraceID, SpanID}`.
- **Invocation-scoped cleanup**: `AfterRunCallback` cleans abandoned state
  without synthesizing terminal metrics.
- **Plugin-first ordering**: register otelgenai first for strongest
  terminal-metric completeness.
- **No default content capture**: prompts, responses, tool arguments, and
  tool results are never recorded.

## Provider Adapters

The following provider adapters are also available:

| Provider | Module | SDK |
|----------|--------|-----|
| OpenAI | `instrumentation/openai` | `github.com/openai/openai-go` |
| Anthropic | `instrumentation/anthropic` | `github.com/anthropics/anthropic-sdk-go` |
| Google GenAI | `instrumentation/google-genai` | `google.golang.org/genai` |
| MCP | `instrumentation/mcp` | `github.com/modelcontextprotocol/go-sdk` |
