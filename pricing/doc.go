// Package pricing provides an optional estimated-cost derivation layer
// for GenAI token usage.
//
// The pricing layer interprets provider-reported usage into billing
// components under the configured pricing semantics. It does not
// reinterpret or replace provider-reported usage semantics; it applies
// the configured price schedule to the usage values recorded on the
// inference span.
//
// Pricing is opt-in: if no [PricingResolver] is configured on the
// [otelgenai.Instrumenter], no cost telemetry is emitted. When a
// resolver is configured, estimated cost is recorded as a span
// attribute only (gen_ai.usage.estimated_cost, float64, USD). No new
// metric instrument is introduced.
//
// Cost is documented as estimated, not authoritative billing. Upstream
// cost conventions are still under active development; this repository
// therefore uses a repository-defined span attribute rather than
// adopting an unstable upstream cost convention.
//
// # Token accounting model
//
// Per the current OTel GenAI conventions, both
// gen_ai.usage.cache_read.input_tokens and
// gen_ai.usage.cache_write.input_tokens are subsets of aggregate
// gen_ai.usage.input_tokens, not additional quantities. The
// [Normalize] function enforces this by subtracting both cache subsets
// from aggregate input to produce non-cached input:
//
//	nonCached = input - cacheRead - cacheWrite
//
// The repository follows the pinned GenAI semantic-convention snapshot
// used by this release. In that snapshot, the cache-write usage field
// is represented as gen_ai.usage.cache_write.input_tokens. Some
// OpenTelemetry/provider surfaces use cache_creation terminology
// (e.g., the Anthropic API exposes CacheCreationInputTokens, which the
// Anthropic adapter maps to CacheWriteTokens). This repository's Go
// field is therefore CacheWriteTokens, aligning with the pinned
// snapshot.
//
// # Disjointness assumption
//
// The v0.4 pricing model assumes cache-read and cache-write counts
// represent disjoint billable components within aggregate input
// tokens. If the reported detailed counts cannot be partitioned within
// aggregate input_tokens, pricing is treated as inconsistent and
// omitted.
//
// # Failure isolation
//
// Cost calculation never fails the inference operation. A missing,
// failed, or inconsistent pricing lookup drops the cost attribute
// silently. Resolver panics are caught by the safety subsystem and
// reported as diagnostics; the original inference result or error is
// preserved unchanged.
package pricing
