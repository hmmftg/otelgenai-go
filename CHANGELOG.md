# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for stable releases. While the major version is `0`, breaking changes may occur
in minor releases because the GenAI semantic conventions are still in
development.

## [Unreleased]

### Added (v0.3)

- MCP adapter module (`instrumentation/mcp`) providing instrumentation for
  the Model Context Protocol using the official
  `github.com/modelcontextprotocol/go-sdk` (v1.7.0).
  - Client-side `AddSendingMiddleware` and server-side
    `AddReceivingMiddleware` instrumentation.
  - Instruments `tools/call`, `resources/read`, and `prompts/get` methods
    as repository-level INTERNAL GenAI operations.
  - W3C trace-context propagation via MCP `_meta` (traceparent, tracestate,
    baggage) using the application's configured global propagator.
  - Server-side agent counter isolation: server middleware extracts only
    remote trace context, preventing local agent counter increments.
  - Metadata ownership: `_meta` is cloned before injection; caller-owned
    maps are never mutated.
  - Non-instrumented methods pass through completely unchanged.
- Core: `InternalOperation` type with `StartInternalOperation` for
  generic INTERNAL spans with configured error classification, nil-safe
    and idempotent `End`.
- Core: `ToolRequest.Attrs` field for adapter-supplied attributes (used by
  MCP for `mcp.method.name`).
- StreamState hardening tests: nil-safe, post-finalization, disabled
  instrumenter.
- Error classification consistency tests across Inference, Tool, and
  InternalOperation.
- Context propagation tests across Agent, Inference, Tool, and
  InternalOperation hierarchies.
- Downstream consumer test for MCP-only dependency isolation.

### Added (v0.2)

- Generic `TraceInference` and `TraceAgent` helpers for provider-neutral
  inference and agent instrumentation.
- Public `testutil` package with recorder and semantic assertions
  (replaces the internal `conformancetest` package).
- OpenAI-compatible provider support via `WithSystem` option on OpenAI
  adapters (Ollama, vLLM, Groq, etc.).
- Google GenAI adapter module (`instrumentation/google-genai`) covering
  `GenerateContent` and `GenerateContentStream` with backend-aware
  attribution (`gcp.gemini`, `gcp.vertex_ai`, `gcp.gen_ai`).
- Documentation: `docs/conventions.md`, `MIGRATION.md`, `docs/roadmap.md`.

### Added (v0.1)

- Initial public release of the framework-neutral OpenTelemetry GenAI
  instrumentation core.
- OpenAI adapter covering Responses, Chat Completions, and Embeddings with
  buffered and streaming instrumentation.
- Anthropic adapter covering Messages with buffered and streaming
  instrumentation.
- Safe-by-default telemetry: no prompts, responses, credentials, cookies,
  or raw bodies are captured unless an explicit content projector is
  configured.
- One logical span per provider operation, enclosing all automatic SDK
  retries.
- Streaming instrumentation with time-to-first-chunk and per-output-chunk
  timing.
- Local `invoke_agent` and client-side `execute_tool` spans and metrics.
- Semantic-convention conformance and sensitive-data contract tests.

### Changed (v0.2)

- Removed `internal/conformancetest` package; replaced by public
  `testutil` package.

[Unreleased]: https://github.com/hmmftg/otelgenai-go/releases
