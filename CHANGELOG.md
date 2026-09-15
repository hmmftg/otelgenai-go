# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for stable releases. While the major version is `0`, breaking changes may occur
in minor releases because the GenAI semantic conventions are still in
development.

## [Unreleased]

### Added (v0.6 hardening)

- Exported well-known operation and system constants (`OperationChat`,
  `OperationGenerateContent`, `OperationTextCompletion`,
  `OperationEmbeddings`, `OperationInvokeAgent`, `OperationExecuteTool`,
  `SystemOpenAI`, `SystemAnthropic`) so adapters no longer import
  `internal/semconv` for these values.
- `Instrumenter.ApplySpanOutcome(ctx, err)`: context-based semantic
  helper that classifies an error and applies `error.type` and status
  to the active span. Replaces raw `trace.Span` mutation in adapters.
- `Instrumenter.AugmentToolSpan(ctx, ToolSpanAttributes)`: context-based
  semantic helper that applies `gen_ai.tool.type` (default `function`)
  and `gen_ai.system` to the active execute_tool span.
- `Version` constant (`0.6.0`) used as the default instrumentation
  scope version for traces, metrics, and logs.
- `gen_ai.conversation.id` is now attached to `InternalOperation`
  spans when present in context, matching inference, agent, and tool
  spans.
- ADK-local `guardedCall` panic-isolation helper so the ADK adapter
  does not depend on `internal/safety`.
- CI: OS × module test matrix (Linux + Windows, all six modules).
- CI: internal-import boundary check that fails when non-test adapter
  files import `internal/*`.
- CI: `govulncheck` extended to all six modules.
- Release: strict version format validation, root tag matches `Version`
  constant, adapter releases validate with `GOWORK=off` against the
  published core.

### Changed (v0.6 hardening)

- All core operation `End` methods (inference, agent, tool, internal)
  now route error classification through the panic-isolated
  `ClassifyError`, removing the unguarded private `classifyError`.
- All adapter `go.mod` files now require core `v0.6.0`.
- All production adapter code removed imports of `internal/semconv`
  and `internal/safety`; test-only imports remain.
- Default instrumentation scope version changed from `0.1.0` to
  `0.6.0`.

### Added (v0.6)

- Correlated OTel log-based events through the Logs API
  (`go.opentelemetry.io/otel/log` and `otel/sdk/log` pinned to
  `v0.22.0`, aligned with the repository's OTel `v1.46.0` baseline).
  - `WithLoggerProvider` option: explicit provider wins; absent, the
    global LoggerProvider is used; a global no-op provider means no
    events; explicit nil is rejected.
  - `WithConversationID` / `ConversationIDFromContext`: canonical
    conversation correlation attached to spans and events as
    `gen_ai.conversation.id`; never added to metrics.
  - `ProjectedContent`: opaque, bounded, non-forgeable projection
    produced only by `Instrumenter.ProjectContent`; encoded separately
    for span JSON attributes and structured Logs API values.
  - `EmitInferenceDetails`: upstream-standard
    `gen_ai.client.inference.operation.details` event with the
    supported subset (operation, provider, request/response model,
    conversation ID, streaming flag, finish reasons, aggregate usage,
    projected system instructions / input / output messages,
    `error.type`). Requires `gen_ai.provider.name` and at least one
    valid projected content field.
  - `EmitToolDetails`: repository-owned
    `otelgenai.execute_tool.operation.details` event with projected
    tool arguments/result or error classification.
  - `EmitAgentOccurrence`: repository-owned occurrence events
    (`otelgenai.agent.model.error_observed`,
    `otelgenai.agent.tool.error_observed`,
    `otelgenai.agent.continued_after_error`). Continuation is an
    observation only; no retry/fallback/recovery claim.
  - Event `Record.Timestamp` is the occurrence time;
    `ObservedTimestamp` is left to the SDK; zero `OccurredAt` captures
    time at emitter entry.
  - `EventsEnabled` / `ContentEventsEnabled` cheap preflights for
    adapters.
  - `Message.Parts` / `MessagePart` / `SystemInstructionParts`
    structured canonical content matching the upstream message-part
    schema (text, tool_call, tool_call_response).
  - `InstrumentationFailureProviderResolverPanic` diagnostic.
- ADK adapter event integration.
  - `WithInferenceProvider` / `WithInferenceProviderResolver`: resolve
    `gen_ai.provider.name` for the standard event; never inferred.
  - Inference details events carry projected request/response content
    and correlate to the enclosing agent trace context (ADK v2.3.0 ends
    `generate_content` before `AfterModel` callbacks run).
  - Tool details events are emitted while the `execute_tool` span is
    still active and carry projected arguments/result.
  - Error-observed and continued-after-error occurrences derived from
    `OnModelError`/`OnToolError` and subsequent `Before*` callbacks.
  - Occurrence timestamps captured at callback entry.
  - Session ID mapped to `gen_ai.conversation.id` on events; never on
    metrics.
  - Metric recordings now use the callback context instead of
    `context.Background()`, preserving trace context and exemplars.
  - No turn spans, duplicate semantic spans, or retry/fallback claims.

### Added (v0.5)

- Google ADK Go framework integration (`instrumentation/adk` module).
  - ADK plugin adapter pinned to `google.golang.org/adk/v2 v2.3.0`.
  - Reuses ADK-native semantic spans (`invoke_agent`, `generate_content`,
    `execute_tool`) without duplication.
  - Agent metrics only (duration, inference count, tool count); no
    adapter-owned agent span attributes.
  - Model metrics only (duration, token usage); no model span mutation.
  - Tool span augmentation with adapter-owned `gen_ai.tool.type` and
    final `error.type`/status while the span is active.
  - Composite `{TraceID, SpanID}` tool state identity prevents
    cross-trace collisions in concurrent runs.
  - Invocation-scoped `AfterRunCallback` cleanup of abandoned lifecycle
    state without synthesizing terminal metrics.
  - Invalid active `execute_tool` span context fails closed: no state,
    no metrics, no parent counter increment.
  - Non-interception: every intercept-capable callback returns
    `(nil, nil)`.
  - Plugin-first ordering recommendation for strongest terminal-metric
    completeness; arbitrary-order limitation documented.
  - `WithSystem` and `WithToolSystemResolver` options.
  - No default content capture.
- Core: exported adapter-facing APIs for framework integrations.
  - `ClassifyError(err error) ErrorType` with panic isolation;
    `ClassifyError(nil)` returns `ErrorTypeNone`.
  - `ReportInstrumentationFailure(f InstrumentationFailure)` bounded
    low-cardinality diagnostic reporting.
  - `RecordInferenceUsage`, `RecordInferenceDuration`,
    `RecordAgentDuration`, `RecordAgentInferenceCalls`,
    `RecordAgentToolCalls`, `RecordToolDuration` with semantic
    arguments (not arbitrary `attribute.KeyValue`).
  - Existing metric semantics preserved: zero agent counts recorded
    once, only positive token components produce measurements,
    negative values fail closed with diagnostics.

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
