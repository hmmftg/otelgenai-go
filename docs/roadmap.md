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

## v0.6 (implemented)

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

## v0.7 (release hardening — candidate)

Scope derived from `docs/technical-review.md` (findings F1–F11):

- Fix adapter `go.mod` core-version requirements; define release order
  (core first, adapters second) and add a no-replace consumer check
- Decide and enforce the policy on adapters importing core `internal/*`
  packages (export through the adapter API, or version-lock releases)
- Stamp the instrumentation scope version per release (replaces the
  hardcoded `0.1.0`)
- Route all core error-classifier call sites through the guarded
  implementation (parity with public `ClassifyError`)
- Close CI gaps: per-module tests on the Windows leg, `govulncheck`
  across all modules
- Docs-consistency pass: README compatibility table (MCP, ADK, log
  dependency), `doc.go` compilable example, `conventions.md`
  cache-token key names
- ADK polish: conversation ID on `StartInternalOperation` spans
  (exemplar-preserving metric contexts and `continued_after_error` for
  tool calls landed in v0.6)

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
