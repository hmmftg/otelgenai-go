# Roadmap

This document describes the planned direction for otelgenai-go. It is
not a firm release commitment; features and timelines may change based
on adoption feedback and upstream semantic-convention evolution.

## v0.2

- Generic `TraceInference` and `TraceAgent` helpers
- Public `testutil` package with recorder and semantic assertions
- OpenAI-compatible provider support via `WithSystem`
- Google GenAI adapter (`GenerateContent`, `GenerateContentStream`)

## v0.3 (released)

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

## v0.4 (implemented)

- Optional pricing resolver (`pricing` subpackage) for estimated cost
  derivation from token usage
  - `gen_ai.usage.estimated_cost` span attribute (repository-defined
    extension, not part of the pinned upstream semantic-convention
    contract)
  - Overflow-safe subset accounting for cache-read and cache-write
    tokens (subtraction-based validation)
  - Price validation (`ValidatePrice`) rejects negative, NaN, Inf
  - Cost validation (`ValidCost`) rejects NaN, Inf from float64 overflow
  - `StaticResolver` with map-copy ownership semantics
  - Resolver panic isolation via `safety.GuardedCallValue`
  - No new metric instrument (deferred until upstream stabilizes)
- testutil cardinality/safety validation helpers (heuristic detectors
  for regression testing)
- Cross-adapter error classification contract documentation
- `Usage` Godoc corrected: "provider-reported token usage" (not
  "billable usage")

## v0.5 (implemented)

- Google ADK Go framework integration (`instrumentation/adk` module)
  - ADK plugin adapter pinned to `google.golang.org/adk/v2 v2.3.0`
  - Reuses ADK-native semantic spans without duplication
  - Agent metrics only; model metrics only; tool span augmentation
  - Composite `{TraceID, SpanID}` tool state identity
  - Invocation-scoped `AfterRunCallback` cleanup
  - Non-interception: every intercept-capable callback returns `(nil, nil)`
  - Plugin-first ordering recommendation; arbitrary-order limitation
  - No default content capture
- Core: exported adapter-facing APIs
  - `ClassifyError`, `ReportInstrumentationFailure`
  - `RecordInferenceUsage`, `RecordInferenceDuration`,
    `RecordAgentDuration`, `RecordAgentInferenceCalls`,
    `RecordAgentToolCalls`, `RecordToolDuration`
  - Semantic-argument API (not arbitrary `attribute.KeyValue`)
  - Existing metric semantics preserved

## v0.6 (implemented; release hardening)

- Correlated events via the OTel Logs API (`WithLoggerProvider`,
  `EventsEnabled`, `ContentEventsEnabled` preflights)
  - Upstream-standard `gen_ai.client.inference.operation.details`
    (opt-in: emitted only when projected content exists; requires a
    resolvable `gen_ai.provider.name`)
  - Repository-owned `otelgenai.execute_tool.operation.details`
  - Repository-owned agent occurrence events
    (`otelgenai.agent.{model,tool}.error_observed`,
    `otelgenai.agent.continued_after_error`) — observations only, no
    inferred retry/fallback/recovery semantics
- Conversation correlation: `WithConversationID` /
  `ConversationIDFromContext` emitting `gen_ai.conversation.id` on
  spans and events, never metrics
- `ProjectedContent` opaque type + `Instrumenter.ProjectContent`;
  validated, bounded, deep-owned projections encodable as both span
  JSON attributes and structured log attribute values
- Canonical `MessagePart` schema (text, tool_call, tool_call_response)
  aligned with the upstream message-part model
- ADK event wiring: provider resolver, projected request/tool content,
  pending-error tracking for `continued_after_error`
- `testutil` log recorder (`LoggerProvider`, `Records`,
  `RecordsWithEventName`)
- Release hardening (findings F1–F6, F10 from technical review):
  - Adapter `go.mod` files require core `v0.6.0`; release workflow
    validates adapters against the published core through sanitized
    no-replace module copies (`scripts/validate-no-replace.sh`)
  - Public diagnostic contract: `Diagnostic`, `DiagnosticReason`,
    `DiagnosticHandler`, and fixed `DiagnosticReason*` constants
    exported from core (previously `WithDiagnosticHandler` took an
    `internal/safety` type unusable by external consumers)
  - `ConversationIDFromContext` returns `(string, bool)`;
    `WithConversationID(ctx, "")` explicitly clears/shadows an
    inherited ID
  - Exported well-known operation/system constants and semantic span
    helpers (`ApplySpanOutcome`, `AugmentToolSpan`); production
    adapter code no longer imports `internal/*`
  - `Version` constant (`0.6.0`) as default instrumentation scope
    version
  - All core `End` methods route through panic-isolated `ClassifyError`
  - `gen_ai.conversation.id` on `StartInternalOperation` spans
  - CI: OS × module test matrix, internal-import boundary check,
    `govulncheck` on all modules
  - Docs: README compatibility table, `doc.go` example, conventions
    cache-token keys and event attributes
- Release guards: strict version format, tag absence (local and
  remote), clean source on `main`, root tag matches `Version`,
  adapter declared core requirement is a published tag
- External published-consumer verification
  (`scripts/verify-published-consumer.sh`)

## v0.7 (planned: reliability and semantic stabilization)

No new telemetry surface. Harden the v0.6 boundaries before v1.0.

- Event exactness: per-event enablement/preflight (inference details,
  tool details, occurrences checked independently); disabled event
  types must not trigger projection work; per-event projector
  error/panic/oversize/invalid-value tests; no raw fallback anywhere
- Correlation matrix: conversation ID propagation verified across
  inference/agent/tool/internal spans and events, plus metric
  exclusion tests
- ADK continuation state machine: exactly-once
  `continued_after_error` for error→model and error→tool; nested and
  sequential child operations; no pending-state leakage between
  invocations; abandoned-invocation cleanup emits nothing synthetic
- Bounded diagnostics: classification, dedup/noise policy, disabled
  instrumentation, handler panic isolation, fixed low-cardinality
  reasons
- Projection ceilings: non-bypassable hard limits enforced during
  traversal/construction (field size, nesting depth, retained event
  payload, item count); numeric values decided after measurement
- Streaming lifecycle audit: terminal success/error, cancel, EOF,
  Close, abandoned stream (caller-owned), partial callbacks,
  duplicate finalization, no post-finalization metrics; fuzz/property
  seeds where useful
- Semantic-convention matrix (documentation-first): classify each
  field as upstream, repository-owned, or legacy divergence; no broad
  renames (F9 decision deferred to v1.0)
- Adapter-family conformance suites (not one universal contract):
  core conformance + provider-wrapper contracts + MCP-specific +
  ADK-specific
- Per-module Go floors validated against the pinned dependency graph
  (hypothesis: Go 1.25 core/provider/MCP, Go 1.26.6 ADK — validate
  before changing `go.mod`), CI matrix to match
- Release reproducibility: dependency-diff checks, tag/version/source
  consistency, fuzz smoke where practical; SBOM/provenance deferred
  (source Go modules, not built artifacts)

Explicitly deferred from v0.7: turn/session abstractions,
retry/recovery inference, automatic fallback detection, conversation
reconstruction, arbitrary event APIs, generic adapter attribute
injection, provider-specific telemetry in core, automatic content
capture, new high-cardinality metrics.

## v1.0 (planned direction)

- Stable semantic-convention contracts
- Public semantic-convention package
- Compatibility matrix and long-term support
- Convention migration decision: span-side `gen_ai.system` →
  `gen_ai.provider.name` and `gen_ai.request.streaming` →
  `gen_ai.request.stream` alignment with the event schema
  (see technical review F9)
- Canonical MCP CLIENT/SERVER RPC span semantics (pending upstream
  convention stabilization for protocol 2026-07-28)
- Stability decision for repository-owned `otelgenai.*` event names

## Backlog (candidate work)

These items are candidates for future releases but are not committed to
any specific version:

- Content redaction/hash utilities (note: redaction currently lives in
  the user-supplied `ContentProjector`; library helpers would raise the
  bar)
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
