# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for stable releases. While the major version is `0`, breaking changes may occur
in minor releases because the GenAI semantic conventions are still in
development.

## [Unreleased]

### Added (v0.4)

- Optional pricing resolver (`pricing` subpackage) for estimated cost
  derivation from token usage, recorded as a span attribute only
  (`gen_ai.usage.estimated_cost`, float64, USD).
  - `Price`, `ModelPricingKey`, `PricingResolver`, `PriceableUsage` types.
  - `Normalize` validates aggregate/subset consistency with overflow-safe,
    subtraction-based validation (cache-read and cache-write are subsets of
    aggregate input).
  - `Estimate` computes cost from normalized usage and price.
  - `ValidatePrice` rejects negative, NaN, and Inf price values.
  - `ValidCost` rejects NaN and Inf computed costs from float64 overflow.
  - `StaticResolver` with map-copy ownership semantics.
  - `WithPricingResolver` option on `Instrumenter`.
  - Resolver panic isolation via `safety.GuardedCallValue` with
    `ReasonResolverPanic` diagnostic.
  - No new metric instrument (deferred until upstream stabilizes).
- testutil cardinality/safety validation helpers: `AssertLowCardinalityModel`,
  `AssertNoDynamicSpanNames`, `AssertNoRawErrors`,
  `AssertNoResourceURIsInSpanNames`, `AssertNoContentInAttributes`
  (heuristic detectors for regression testing, not formal cardinality
  analysis).
- testutil cost assertions: `AssertEstimatedCost`, `AssertNoEstimatedCost`.
- Cross-adapter error classification contract documentation
  (`docs/error_contract.md`).

### Changed (v0.4)

- `Usage` Godoc updated from "provider-reported billable usage; the library
  never estimates" to "provider-reported token usage. The library does not
  infer missing usage values."
- `docs/roadmap.md` updated: v0.3 marked as released, v0.4 section expanded.

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
