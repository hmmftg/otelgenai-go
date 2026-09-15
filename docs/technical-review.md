# Technical Review — September 2026

Static review of the repository covering implemented roadmap items (v0.2–v0.5),
in-flight event/correlation work, and release readiness. This document is the
input for v0.6/v0.7 planning; findings are ordered by severity.

## Status snapshot

| Milestone | Status |
|-----------|--------|
| v0.2 (generic helpers, testutil, WithSystem, google-genai) | Implemented |
| v0.3 (MCP adapter, InternalOperation, StreamState hardening) | Implemented |
| v0.4 (pricing resolver, cost attribute, cardinality testutil) | Implemented |
| v0.5 (ADK plugin, exported adapter APIs) | Implemented, pending release |
| v0.6 (correlated events, conversation ID, ProjectedContent) | In progress (uncommitted) |
| v1.0 (stable contracts, public semconv, LTS) | Planned |

## Design properties to preserve

These are deliberate decisions; regressions here are defects, not style issues.

1. **Metadata-only by default.** No prompts, responses, tool arguments,
   results, credentials, or raw error messages without an explicit
   `ContentProjector`. Every projection failure (error, panic, invalid shape,
   oversize) drops the projection with no raw fallback (`content.go`).
2. **One logical span per provider operation**, enclosing all SDK retries.
   Adapters wrap typed SDK services; provider types never leak into the core
   API.
3. **Panic isolation for every user-supplied callback** (projector,
   classifier, resolvers, diagnostic handler) via `safety.GuardedCall*`.
4. **Constrained adapter-facing API.** Framework adapters record telemetry
   through semantic-argument methods (`RecordInferenceUsage`,
   `ClassifyError`, `ReportInstrumentationFailure`), not arbitrary
   `attribute.KeyValue`. Attribute construction and cardinality stay in core.
5. **MCP propagation order:** start the client span first, then inject the
   span-enriched context into a cloned `_meta`. Caller-owned metadata is
   never mutated.
6. **Bounded diagnostics.** `InstrumentationFailure` is a fixed enum; no raw
   errors or content ever reach diagnostic handlers.
7. **Stream finalization on every observable terminal path** (terminal
   event, error, cancellation, EOF, Close); callers own abandoned streams.
8. **Repo-owned event names use the `otelgenai.*` namespace** and are static;
   dynamic identity (tool name, conversation ID, error type) belongs in
   attributes. Occurrence events record observations only — never inferred
   retry/fallback/recovery semantics.
9. **`gen_ai.conversation.id` on traces and events only, never metrics**
   (cardinality). The library never synthesizes conversation IDs.

## Findings

### F1 — Critical: adapter modules declare the wrong core version

Every `instrumentation/*/go.mod` requires `github.com/hmmftg/otelgenai-go
v0.1.0` and resolves it via `replace => ../..`. Replace directives in
dependency modules do not propagate to consumers, so published adapter tags
will compile against core v0.1.0, which lacks:

- `StartInternalOperation` (used by MCP, added v0.3)
- `ClassifyError`, `Record*`, `ReportInstrumentationFailure` (used by ADK, v0.5)
- `ProjectContent`, `Emit*`, `WithConversationID` (used by ADK, v0.6)

The downstream consumer tests do not catch this because they `replace` both
the core and the adapter to local paths (`test/downstream/*/go.mod`).

**Action:** before tagging, bump each adapter's `require` to the next core
version; establish release order (core first, then adapters); add one
no-replace consumer test that resolves versions from the proxy (e.g. in the
release workflow against a staged tag, or via `go mod download` checks).

### F2 — High: ADK adapter now imports core internal packages

`instrumentation/adk/tool.go` and `inference.go` import
`internal/semconv` and `internal/safety`. Legal (shared module-path prefix),
but it couples a separately versioned module to core internals: any internal
refactor silently breaks the published adapter, and it bypasses the
constrained adapter API boundary created in v0.5.

**Action:** decide the policy explicitly. Either (a) export what adapters
need through the adapter API (preferred — e.g. a guarded-call helper and a
tool-attribute setter like `SetToolSpanAttributes`), or (b) accept internal
imports and version-lock adapter releases to exact core versions.

### F3 — Medium: instrumentation scope version is hardcoded `0.1.0`

`defaultInstrumentationVersion` in `option.go` still reports 0.1.0 for all
releases unless users set `WithInstrumentationVersion`. Telemetry scope
metadata will mislead compatibility debugging.

**Action:** stamp the version at release (linker flag or generated
`version.go`), or bump the constant per release as a required checklist item.

### F4 — Medium: unguarded error classifier in core paths

`Instrumenter.classifyError` calls `in.classifier(err)` directly; it is used
by `InferenceOperation.End`, `AgentOperation.End`, `ToolOperation.End`, and
`InternalOperation.End`. A panicking user classifier crashes the app on the
hot path — contradicting the safety package contract. The public
`ClassifyError` (v0.5) is guarded; the private path is not.

**Action:** route all internal call sites through the guarded implementation
(single source of truth), keeping `ErrorTypeNone → ErrorTypeUnknown`
normalization.

### F5 — Medium: CI coverage gaps

- Root `go build ./... && go test ./...` does not cover nested modules; only
  the Ubuntu race job tests each adapter. Adapter tests never run on Windows.
- `govulncheck` covers root, openai, anthropic only — not google-genai, mcp,
  or adk.
- Downstream builds use `replace` for both core and adapter (see F1).

**Action:** run per-module test/build on both OS matrix legs; extend
govulncheck to all modules; add the no-replace consumer check from F1.

### F6 — Medium: documentation drift

- `docs/roadmap.md` labels both v0.4 and v0.5 "(current)".
- README compatibility table omits the MCP and ADK modules.
- `doc.go` example references `otelgenai.OperationChat`, but operation
  constants live in `internal/semconv` — the snippet does not compile as
  written.
- `docs/conventions.md` documents `gen_ai.usage.cache_read_tokens`; the
  implementation emits `gen_ai.usage.cache_read_input_tokens` /
  `..._write_input_tokens` (span names) and event-only
  `cache_read.input_tokens` / `reasoning.output_tokens` variants.
- README claims the core depends only on OTel API packages; the root module
  now also carries `otel/sdk/log` for `testutil`. Update the claim or split
  testutil into its own module.

**Action:** single docs-consistency pass in the release checklist.

### F7 — Low: lost metric exemplar context in ADK agent/tool paths

`adk/agent.go` (`afterAgent`) and `adk/tool.go` (`afterTool`) record metrics
with `context.Background()`; `adk/inference.go` now passes `ctx`. Metrics
recorded on Background lose trace-context exemplar links.

**Action:** prefer the callback context (or stored span context) for all
metric recording where one is available.

### F8 — Low: `continued_after_error` only fires before model calls

`registry.takePendingError` is consumed in `beforeModel` only. The occurrence
contract says "a subsequent model or tool call"; `beforeTool` does not check
the pending error, so error → tool-call sequences never produce the event.

**Action:** either consume `pendingError` in `beforeTool` too, or narrow the
documented semantics to "subsequent model call".

### F9 — Low: span vs event attribute naming divergence needs a v1.0 decision

Spans use `gen_ai.system` / `gen_ai.request.streaming`; the new events use
`gen_ai.provider.name` / `gen_ai.request.stream` (event schema). This mirrors
upstream's rename, but it means the same concept has two names across
signals. `internal/semconv` documents the divergence; a migration or
dual-emission decision is required before v1.0 stabilizes the contract.

### F10 — Low: `gen_ai.conversation.id` missing on `StartInternalOperation` spans

Conversation correlation was added to inference, agent, and tool spans.
`InternalOperation` (used by MCP `resources/read`/`prompts/get`) does not
carry it. If conversation correlation is part of the v0.6 contract, add it
there or document the exclusion.

### F11 — Info: `go 1.27` floor restricts adoption

All modules require Go 1.27 (needed for `iter.Seq2`-era APIs and current OTel
SDKs). Acceptable for greenfield users; narrows the consumer base. Re-evaluate
against the lowest Go version the pinned dependencies actually require when
v1.0 compatibility policy is written.

## Guardrails for future implementation

Checklist distilled from the findings; apply to every new feature/adapter:

- [ ] New adapter-facing core capability ⇒ bump every consuming adapter's
      `require` line to the next core version in the same change.
- [ ] Never add a `replace` for a library dependency expecting it to affect
      consumers; test consumption without replaces.
- [ ] Every user-supplied callback (projector, classifier, resolver,
      handler) executes under `safety.GuardedCall*`; no raw values in
      diagnostics.
- [ ] Content capture stays opt-in, bounded by `projectionLimit`, and
      fail-closed. `ProjectedContent` stays opaque — callers cannot mint
      unvalidated content.
- [ ] Conversation/session IDs and user identifiers never appear on metrics.
- [ ] Event names are static and namespaced (`gen_ai.*` only for true
      upstream-standard events; everything else `otelgenai.*`).
- [ ] Adapters report instrumentation failures via the bounded enum, not
      error strings.
- [ ] Adapters do not import `internal/*` until F2's policy decision is made.
- [ ] Convention name changes go through `internal/semconv` only; document
      span/event divergences in `docs/conventions.md`.
- [ ] Update README compatibility table and CHANGELOG in the same PR as any
      new module or pinned-version change.
