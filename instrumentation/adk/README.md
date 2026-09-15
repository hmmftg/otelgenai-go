# ADK Go Instrumentation

OpenTelemetry GenAI instrumentation for [Google ADK Go](https://github.com/google/adk-go) v2.3.0.

## Overview

This module provides an ADK plugin that instruments agent, model, and tool
lifecycle through the otelgenai core APIs. It reuses ADK's native semantic
spans (`invoke_agent`, `generate_content`, `execute_tool`) without creating
duplicates, and emits existing otelgenai metrics through constrained core APIs.

## Installation

```bash
go get github.com/hmmftg/otelgenai-go/instrumentation/adk
```

## Quick Start

```go
package main

import (
    "github.com/hmmftg/otelgenai-go"
    adkadapter "github.com/hmmftg/otelgenai-go/instrumentation/adk"
    "google.golang.org/adk/v2/runner"
)

func main() {
    instr, err := otelgenai.New()
    if err != nil {
        panic(err)
    }

    plugin, err := adkadapter.New(instr)
    if err != nil {
        panic(err)
    }

    // Register the plugin with your ADK runner.
    // Register otelgenai first for strongest terminal-metric completeness.
    r, err := runner.New(runner.Config{
        // ...
        Plugins: []*plugin.Plugin{plugin},
    })
    if err != nil {
        panic(err)
    }
    _ = r
}
```

## Architecture

### Native Span Reuse

The adapter creates no semantic spans. ADK owns:

```
invoke_agent
  ├── generate_content
  │     └── provider adapter span (optional)
  └── execute_tool
        └── MCP/lower-level operation (optional)
```

The adapter:
- Observes `invoke_agent` without mutation (agent metrics only).
- Never mutates `generate_content` (model metrics only).
- Augments active `execute_tool` with adapter-owned `gen_ai.tool.type` and
  final `error.type`/status while the span is active.

### Callback Identity

ADK v2.3.0 plugin callbacks expose `InvocationID()`, `Branch()`, and
`AgentName()`. The adapter identity is:

```
agentKey = InvocationID + Branch + AgentName
```

### Tool Identity

Tool state is keyed by the composite `{TraceID, SpanID}` of the active
`execute_tool` span. A `SpanID` alone is only unique within its trace; the
composite key prevents cross-trace collisions in concurrent runs. Invalid
span contexts fail closed: no state, no metrics, no parent counter increment.

### Non-Interception

Every intercept-capable adapter callback returns `(nil, nil)`, never a
replacement response/result/content, and never an error solely for telemetry
purposes. `AfterRunCallback` is teardown-only and returns nothing.

### Cross-Plugin Ordering

Register the otelgenai plugin **first** for strongest terminal-metric
completeness. If a later plugin intercepts `Before*` after otelgenai created
state, the native operation may be skipped and no matching terminal callback
arrives. `AfterRunCallback` performs invocation-scoped cleanup of abandoned
state without synthesizing terminal metrics.

If otelgenai is registered after a plugin that intercepts `After*`, terminal
telemetry may be missing; this is a documented limitation.

## Options

### WithSystem

Sets the `gen_ai.system` value used for inference metric dimensions.
Defaults to `"adk"`.

```go
adkadapter.New(instr, adkadapter.WithSystem("gemini"))
```

### WithToolSystemResolver

Sets a resolver that maps an ADK tool to a `gen_ai.system` value for the
`execute_tool` span. When unset or when the resolver returns an empty string,
`gen_ai.system` is omitted. Resolver panics are recovered; the attribute is
omitted and a diagnostic is reported.

```go
adkadapter.New(instr, adkadapter.WithToolSystemResolver(func(t toolNameProvider) string {
    return "custom-system"
}))
```

### WithInferenceProvider

Sets the `gen_ai.provider.name` reported on the standard
`gen_ai.client.inference.operation.details` event. The upstream event schema
requires a provider name; without one the event is not emitted. ADK is a
framework, not a provider, so this value is never inferred from model names.

```go
adkadapter.New(instr, adkadapter.WithInferenceProvider("gcp.vertex_ai"))
```

### WithInferenceProviderResolver

Sets a resolver that maps a request model name to a `gen_ai.provider.name`
value. A non-empty resolver result takes precedence over the static provider;
an empty result falls back to it. Resolver panics are isolated and reported
as diagnostics.

```go
adkadapter.New(instr, adkadapter.WithInferenceProviderResolver(func(model string) string {
    return "gcp.vertex_ai"
}))
```

## Correlated Events

When the instrumenter has a `ContentProjector` configured and its logger
provider accepts records, the adapter emits:

- `gen_ai.client.inference.operation.details` — upstream-standard event with
  projected system instructions, input/output messages, models, usage, finish
  reasons, streaming flag, and `gen_ai.conversation.id` derived from the ADK
  session ID. Requires a provider name (see above).
- `otelgenai.execute_tool.operation.details` — repository-owned event with
  projected tool arguments/result, emitted while the `execute_tool` span is
  still active so the event is correlated to that span.

Without a projector, no content-bearing events are emitted. Because ADK
v2.3.0 ends the `generate_content` span before `AfterModel` callbacks run,
inference details events correlate to the enclosing agent trace context, not
the ended model span.

The adapter also emits content-free occurrence events:

- `otelgenai.agent.model.error_observed` / `otelgenai.agent.tool.error_observed`
  — an error was observed by the framework error callback.
- `otelgenai.agent.continued_after_error` — the agent started another model or
  tool call after a previously observed error. This is an observation, not a
  claim about retry, fallback, or recovery.

Event timestamps are occurrence times captured at callback entry. The session
ID is used only for trace/log correlation and is never added to metrics.

ADK's own opt-in content capture
(`OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT`) is outside the
adapter's projector boundary and may coexist with these events.

## Limitations

- Unknown/streaming tool paths without the ordinary callback sequence cannot
  be fully counted.
- `agenttool`-created nested runners may not inherit `PluginConfig` in v2.3.0
  (ADK issue #669).
- No workflow-node (`invoke_node`) augmentation; ADK already creates it and
  exposes no public node callback.
- No default content capture; prompts, responses, tool arguments, and tool
  results are never recorded unless a `ContentProjector` is configured and
  the logger accepts events.

## ADK Version

This module is pinned to `google.golang.org/adk/v2 v2.3.0`. Upgrading ADK
must be treated as a deliberate compatibility change.
