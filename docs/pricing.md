# Pricing and Estimated Cost

`otelgenai-go` v0.4 adds an optional pricing resolver that derives
estimated cost from token usage as a span attribute only
(`gen_ai.usage.estimated_cost`, float64, USD). No new metric instrument
is introduced.

## Overview

Pricing is **opt-in**. If no `PricingResolver` is configured on the
`Instrumenter`, no cost telemetry is emitted. When configured, estimated
cost is recorded as a span attribute only.

Cost is documented as **estimated, not authoritative billing**. Upstream
cost conventions are still under active development; v0.4 therefore uses
a repository-defined span attribute rather than adopting an unstable
upstream cost convention.

## Quick start

```go
import (
    "github.com/hmmftg/otelgenai-go"
    "github.com/hmmftg/otelgenai-go/pricing"
)

resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
    {System: "openai", Model: "gpt-4o"}: {
        InputPerToken:  0.00001,
        OutputPerToken: 0.00003,
    },
})

instr, err := otelgenai.New(
    otelgenai.WithPricingResolver(resolver),
)
```

## Token accounting model

The pricing layer interprets provider-reported usage into billing
components under the configured pricing semantics. It does not
reinterpret or replace provider-reported usage semantics; it applies
the configured price schedule to the usage values recorded on the
inference span.

Per the current OTel GenAI conventions, both
`gen_ai.usage.cache_read.input_tokens` and
`gen_ai.usage.cache_write.input_tokens` are **subsets** of aggregate
`gen_ai.usage.input_tokens`, not additional quantities. The `Normalize`
function enforces this by subtracting both cache subsets from aggregate
input:

```
nonCached = input - cacheRead - cacheWrite
```

### Terminology note

The repository follows the pinned GenAI semantic-convention snapshot
used by this release. In that snapshot, the cache-write usage field is
represented as `gen_ai.usage.cache_write.input_tokens`. Some
OpenTelemetry/provider surfaces use `cache_creation` terminology (e.g.,
the Anthropic API exposes `CacheCreationInputTokens`, which the
Anthropic adapter maps to `CacheWriteTokens`). This repository's Go
field is therefore `CacheWriteTokens`, aligning with the pinned
snapshot.

### Disjointness assumption

The v0.4 pricing model assumes cache-read and cache-write counts
represent disjoint billable components within aggregate input tokens.
If the reported detailed counts cannot be partitioned within aggregate
`input_tokens`, pricing is treated as inconsistent and omitted.

```
InputTokens (aggregate, includes both cached subsets)
├── CacheReadTokens  (subset of InputTokens)
├── CacheWriteTokens (subset of InputTokens)
└── NonCachedInputTokens = input - cacheRead - cacheWrite
```

### Validation

`Normalize` uses overflow-safe, subtraction-based validation:

```go
if cacheRead > input {
    return invalid
}
remaining := input - cacheRead
if cacheWrite > remaining {
    return invalid
}
nonCached := remaining - cacheWrite
```

If validation fails, the pricing layer omits cost rather than emitting
a misleading estimate.

## API

### `Price`

```go
type Price struct {
    InputPerToken      float64 // non-cached input
    OutputPerToken     float64
    CacheReadPerToken  float64
    CacheWritePerToken float64
}
```

Zero values mean zero-cost (free) for that token category, NOT
"unknown". Whether pricing is known at all is determined by the
`PricingResolver`'s `(Price, bool)` return: `false` means unknown
pricing; `true` with a zero field means known, free.

### `PricingResolver`

```go
type PricingResolver interface {
    Resolve(key ModelPricingKey) (Price, bool)
}
```

`Resolve` may be called concurrently by multiple inference operations;
implementations must be safe for concurrent use. The resolver is
responsible for billing semantics, including model alias resolution
(e.g., `gpt-5` vs `gpt-5-2026-08-07`).

### `StaticResolver`

`StaticResolver` is a built-in resolver backed by a map. The
constructor copies the input map so subsequent mutations to the
caller's map do not affect the resolver.

```go
resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
    {System: "openai", Model: "gpt-4o"}: {
        InputPerToken:  0.00001,
        OutputPerToken: 0.00003,
    },
})
```

## Failure isolation

Cost calculation never fails the inference operation:

| Scenario | Behavior |
|---|---|
| No resolver configured | No cost attribute; only nil check |
| Unknown model | No cost attribute; no error |
| Invalid price (negative, NaN, Inf) | No cost attribute; no error |
| Inconsistent usage (subsets exceed aggregate) | No cost attribute; no error |
| Resolver panic | Caught by safety subsystem; no cost attribute; inference succeeds |
| Computed cost overflow (+Inf, NaN) | No cost attribute; no error |
| Zero tokens / free model | `cost == 0`; no cost attribute emitted |

## Cost attribute

The `gen_ai.usage.estimated_cost` attribute is a **repository-defined
extension** and is not part of the pinned upstream semantic-convention
contract. Upstream cost semantics are actively evolving (see
[open-telemetry/semantic-conventions-genai#287](https://github.com/open-telemetry/semantic-conventions-genai/issues/287));
this repository intentionally uses a local attribute in v0.4 and may
migrate it in a future compatibility release.

The attribute does not introduce metric cardinality because it is
recorded only on spans. However, every span attribute adds storage
volume and may be indexed differently by downstream systems; this is
an acceptable tradeoff for an opt-in feature.

## Reasoning tokens

Reasoning tokens are excluded as a separate pricing component in v0.4
because the core output token count already includes them per the
current OTel GenAI conventions. A future pricing model may represent
providers that bill reasoning separately.
