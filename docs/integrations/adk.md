# Google ADK Go Integration

The `instrumentation/adk` module provides OpenTelemetry GenAI instrumentation
for [Google ADK Go](https://github.com/google/adk-go) v2.3.0 through the ADK
plugin system.

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

    r, err := runner.New(runner.Config{
        Plugins: []*plugin.Plugin{plugin},
    })
    if err != nil {
        panic(err)
    }
    _ = r
}
```

## Span Hierarchy

The adapter reuses ADK's native semantic spans without duplication:

```
invoke_agent
  ├── generate_content
  │     └── provider adapter span (optional)
  └── execute_tool
        └── MCP/lower-level operation (optional)
```

- **Agent**: observed, not mutated. Agent metrics only.
- **Model**: observed, not mutated. Model metrics only.
- **Tool**: augmented with `gen_ai.tool.type` and final `error.type`/status
  while the span is active.

## Metrics

The adapter emits the following existing otelgenai metrics:

| Metric | Instrument | Dimensions |
|--------|-----------|------------|
| Agent duration | `gen_ai.invoke_agent.duration` | agent name |
| Agent inference calls | `gen_ai.invoke_agent.inference_calls` | agent name |
| Agent tool calls | `gen_ai.invoke_agent.tool_calls` | agent name |
| Inference duration | `gen_ai.client.operation.duration` | operation, system, model |
| Token usage | `gen_ai.client.token.usage` | operation, system, model, token type |
| Tool duration | `gen_ai.execute_tool.duration` | tool name |

Zero agent counts are recorded once (matching existing core behavior). Only
positive input/output token components produce token usage measurements.

## Plugin Ordering

Register the otelgenai plugin **first** for strongest terminal-metric
completeness. See the [README](../../instrumentation/adk/README.md) for
details on cross-plugin ordering limitations.

## ADK Version

Pinned to `google.golang.org/adk/v2 v2.3.0`. Upgrading ADK must be treated as
a deliberate compatibility change.
