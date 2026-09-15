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

## Limitations

- Unknown/streaming tool paths without the ordinary callback sequence cannot
  be fully counted.
- `agenttool`-created nested runners may not inherit `PluginConfig` in v2.3.0
  (ADK issue #669).
- No workflow-node (`invoke_node`) augmentation; ADK already creates it and
  exposes no public node callback.
- No default content capture; prompts, responses, tool arguments, and tool
  results are never recorded.

## ADK Version

This module is pinned to `google.golang.org/adk/v2 v2.3.0`. Upgrading ADK
must be treated as a deliberate compatibility change.
