# Roadmap

This document describes the planned direction for otelgenai-go. It is
not a firm release commitment; features and timelines may change based
on adoption feedback and upstream semantic-convention evolution.

## v0.2

- Generic `TraceInference` and `TraceAgent` helpers
- Public `testutil` package with recorder and semantic assertions
- OpenAI-compatible provider support via `WithSystem`
- Google GenAI adapter (`GenerateContent`, `GenerateContentStream`)

## v0.3 (current)

- MCP (Model Context Protocol) instrumentation using
  `modelcontextprotocol/go-sdk` (v1.7.0)
  - Client and server middleware for `tools/call`, `resources/read`,
    `prompts/get`
  - W3C trace-context propagation via MCP `_meta`
  - Repository-level INTERNAL operation mapping (not canonical MCP
    CLIENT/SERVER RPC spans)
  - Server-side agent counter isolation
  - Metadata ownership: clone before injection, never mutate caller state
- Core: `InternalOperation` type with `StartInternalOperation`
- Core: `ToolRequest.Attrs` field for adapter-supplied attributes
- StreamState hardening (nil-safe, post-finalization, disabled)
- Error classification consistency tests
- Context propagation tests

## v0.4 (planned direction)

- Cost/pricing tracking for token usage

## v0.5 (planned direction)

- Framework integrations

## v1.0 (planned direction)

- Stable semantic-convention contracts
- Public semantic-convention package
- Compatibility matrix and long-term support
- Canonical MCP CLIENT/SERVER RPC span semantics (pending upstream
  convention stabilization for protocol 2026-07-28)

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
