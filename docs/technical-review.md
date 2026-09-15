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
| v0.5 (ADK plugin, exported adapter APIs) | Implemented |
| v0.6 (correlated events, conversation ID, ProjectedContent) | Implemented; release hardening |
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

**Resolved.** All adapter `go.mod` files now require core `v0.6.0`.
The release workflow validates adapter releases with `GOWORK=off`
against the published core. Local `replace` directives remain for
development until the core tag is published; the release workflow's
adapter path performs the no-replace consumer check.

Original finding: every `instrumentation/*/go.mod` required
`github.com/hmmftg/otelgenai-go v0.1.0` and resolved it via
`replace => ../..`. Replace directives in dependency modules do not
propagate to consumers, so published adapter tags would compile
against core v0.1.0, which lacks post-v0.1 APIs.

### F2 — High: adapter imports of core internal packages

**Resolved.** All production adapter code (openai, anthropic,
google-genai, adk) no longer imports `internal/semconv` or
`internal/safety`. Well-known operation/system constants are exported
from the core package. The ADK adapter uses a local `guardedCall`
helper and the `ApplySpanOutcome` / `AugmentToolSpan` semantic APIs.
Test-only imports of `internal/*` remain. CI enforces the boundary
with a dedicated check.

Original finding: `instrumentation/adk/tool.go` and `inference.go`
imported `internal/semconv` and `internal/safety`, coupling a
separately versioned module to core internals. The issue also
affected `openai`, `anthropic`, and `google-genai` mapping files.

### F3 — Medium: instrumentation scope version is hardcoded `0.1.0`

**Resolved.** A `Version` constant (`"0.6.0"`) is exported and used as
the default instrumentation scope version for traces, metrics, and
logs. The release workflow validates that a root release tag matches
`v${Version}` before tagging.

### F4 — Medium: unguarded error classifier in core paths

**Resolved.** All core operation `End` methods (inference, agent,
tool, internal) now route through the panic-isolated `ClassifyError`.
The unguarded private `classifyError` has been removed. A panicking
classifier reports a `classifier.panic` diagnostic and sets
`error.type=unknown` without crashing `End`.

### F5 — Medium: CI coverage gaps

**Resolved.** CI now uses an OS × module matrix (Linux + Windows,
all six modules) for vet/build/test. `govulncheck` is extended to
all six modules. An internal-import boundary check fails when
non-test adapter files import `internal/*`.

### F6 — Medium: documentation drift

**Resolved.** README compatibility table now includes MCP and ADK
rows. `doc.go` uses the exported `OperationChat` constant.
`docs/conventions.md` documents correct cache-token keys
(`cache_read_input_tokens`, `cache_write_input_tokens`), event-only
attributes, conversation ID, and repository-owned event names.

Original finding: roadmap labelled v0.4/v0.5 "(current)", README
omitted MCP/ADK, `doc.go` referenced an internal constant, and
conventions documented incorrect cache-token keys.

### F7 — Low: lost metric exemplar context in ADK agent/tool paths

**Resolved** (commit `00bb1f0`). ADK agent and tool metric
recordings now use the callback context instead of
`context.Background()`, preserving trace-context exemplar links.

### F8 — Low: `continued_after_error` only fires before model calls

**Resolved** (commit `dd1f1e0`). `beforeTool` now consumes pending
errors and emits `continued_after_error` before subsequent tool calls,
matching the documented "subsequent model or tool call" semantics.

### F9 — Low: span vs event attribute naming divergence needs a v1.0 decision

Spans use `gen_ai.system` / `gen_ai.request.streaming`; the new events use
`gen_ai.provider.name` / `gen_ai.request.stream` (event schema). This mirrors
upstream's rename, but it means the same concept has two names across
signals. `internal/semconv` documents the divergence; a migration or
dual-emission decision is required before v1.0 stabilizes the contract.

### F10 — Low: `gen_ai.conversation.id` missing on `StartInternalOperation` spans

**Resolved.** `StartInternalOperation` now attaches
`gen_ai.conversation.id` from context when present, matching
inference, agent, and tool spans. The caller-supplied attributes
slice is not mutated.

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
- [ ] Adapters do not import `internal/*` in production code (F2 policy:
      export what adapters need through the public API; CI enforces the
      boundary).
- [ ] Convention name changes go through `internal/semconv` only; document
      span/event divergences in `docs/conventions.md`.
- [ ] Update README compatibility table and CHANGELOG in the same PR as any
      new module or pinned-version change.
