# Roadmap

This document describes the planned direction for otelgenai-go. It is
not a firm release commitment; features and timelines may change based
on adoption feedback and upstream semantic-convention evolution.

## v0.2 (current)

- Generic `TraceInference` and `TraceAgent` helpers
- Public `testutil` package with recorder and semantic assertions
- OpenAI-compatible provider support via `WithSystem`
- Google GenAI adapter (`GenerateContent`, `GenerateContentStream`)

## v0.3 (planned direction)

- MCP (Model Context Protocol) instrumentation using
  `modelcontextprotocol/go-sdk`

## v0.4 (planned direction)

- Cost/pricing tracking for token usage

## v0.5 (planned direction)

- Framework integrations

## v1.0 (planned direction)

- Stable semantic-convention contracts
- Public semantic-convention package
- Compatibility matrix and long-term support

## Backlog (candidate work)

These items are candidates for future releases but are not committed to
any specific version:

- Content redaction/hash utilities
- Prompt identity and versioning
- Grafana dashboards
- Aspire integration examples
- Collector integration examples
- Additional provider adapters
- Embedding instrumentation helpers

## Explicitly deferred

The following are intentionally out of scope for the foreseeable future:

- A separate OpenAI-compatible module (handled via `WithSystem` on the
  existing OpenAI adapter)
- Public semantic-convention packages before v1.0
- `InferenceCaller` abstraction (subsumed by `TraceInference`)
- `Wrap` function (subsumed by typed adapter wrappers)
- `TraceEmbedding` (use `TraceInference` with `Request.Operation`)
