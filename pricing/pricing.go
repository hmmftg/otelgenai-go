package pricing

import "math"

// Price holds per-token prices in USD. Zero values mean zero-cost
// (free) for that token category, NOT "unknown". Whether pricing is
// known at all is determined by the [PricingResolver]'s (Price, bool)
// return: false means unknown pricing; true with a zero field means
// known, free.
//
// InputPerToken applies only to input tokens NOT served from or
// written to a provider-managed cache. Cached input is priced
// separately via CacheReadPerToken and CacheWritePerToken to avoid
// double-counting (both cache-read and cache-write tokens are subsets
// of aggregate input tokens per the OTel GenAI conventions).
type Price struct {
	InputPerToken      float64 // non-cached input
	OutputPerToken     float64
	CacheReadPerToken  float64
	CacheWritePerToken float64
}

// ModelPricingKey identifies a model for pricing lookup. The resolver
// decides how to handle aliases (e.g. "gpt-5" vs "gpt-5-2026-08-07").
type ModelPricingKey struct {
	System string // gen_ai.system / gen_ai.provider.name
	Model  string // gen_ai.request.model
}

// PricingResolver maps a (system, model) pair to a [Price]. Returns
// (Price, true) when pricing is known, (Price{}, false) otherwise.
// Resolve may be called concurrently by multiple inference operations;
// implementations must be safe for concurrent use.
//
// The resolver is responsible for billing semantics: the returned
// Price defines how the four billable categories (non-cached input,
// output, cache-read, cache-write) are interpreted. A future
// resolver can encapsulate region, service tier, effective date,
// batch vs realtime, and other pricing dimensions without changing
// the core interface.
type PricingResolver interface {
	Resolve(key ModelPricingKey) (Price, bool)
}

// PriceableUsage holds the four billable token categories after
// normalization. These are NOT the raw provider-reported values;
// they are derived so that no category is a subset of another.
type PriceableUsage struct {
	NonCachedInputTokens int64 // input minus cache-read and cache-write subsets
	OutputTokens         int64
	CacheReadTokens      int64
	CacheWriteTokens     int64
}

// Normalize validates aggregate/subset consistency of provider-reported
// usage and produces non-overlapping token categories under the
// repository's v0.4 pricing model. Both cacheReadTokens and
// cacheWriteTokens are treated as subsets of inputTokens. v0.4 assumes
// cache-read and cache-write are disjoint billing components; if
// reported totals exceed aggregate input, pricing is treated as
// inconsistent and omitted. If the subsets exceed the aggregate
// (cacheRead > input, or cacheWrite > input - cacheRead), returns
// (PriceableUsage{}, false) to signal the caller should omit cost
// rather than emit a misleading estimate.
//
// Validation is overflow-safe: it uses subtraction-based checks
// rather than computing cacheRead + cacheWrite, which could overflow
// int64.
func Normalize(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) (PriceableUsage, bool) {
	// Reject negative usage.
	if inputTokens < 0 || outputTokens < 0 || cacheReadTokens < 0 || cacheWriteTokens < 0 {
		return PriceableUsage{}, false
	}
	// Both cacheRead and cacheWrite are subsets of input. Validate
	// via subtraction to avoid int64 overflow on cacheRead + cacheWrite.
	if cacheReadTokens > inputTokens {
		return PriceableUsage{}, false
	}
	remaining := inputTokens - cacheReadTokens
	if cacheWriteTokens > remaining {
		return PriceableUsage{}, false
	}
	return PriceableUsage{
		NonCachedInputTokens: remaining - cacheWriteTokens,
		OutputTokens:         outputTokens,
		CacheReadTokens:      cacheReadTokens,
		CacheWriteTokens:     cacheWriteTokens,
	}, true
}

// Estimate computes the estimated cost in USD from the given normalized
// usage and price. Estimate is pure; callers should validate the price
// via [ValidatePrice] and the result via [ValidCost] before using the
// returned value.
func Estimate(usage PriceableUsage, price Price) float64 {
	return float64(usage.NonCachedInputTokens)*price.InputPerToken +
		float64(usage.OutputTokens)*price.OutputPerToken +
		float64(usage.CacheReadTokens)*price.CacheReadPerToken +
		float64(usage.CacheWriteTokens)*price.CacheWritePerToken
}

// ValidatePrice reports whether all price fields are finite and
// non-negative. Invalid resolver output (negative, NaN, or Inf)
// is treated exactly like unknown pricing: no cost attribute is
// emitted. This guards against application-supplied resolvers that
// return malformed values.
func ValidatePrice(p Price) bool {
	return p.InputPerToken >= 0 &&
		p.OutputPerToken >= 0 &&
		p.CacheReadPerToken >= 0 &&
		p.CacheWritePerToken >= 0 &&
		!math.IsNaN(p.InputPerToken) &&
		!math.IsNaN(p.OutputPerToken) &&
		!math.IsNaN(p.CacheReadPerToken) &&
		!math.IsNaN(p.CacheWritePerToken) &&
		!math.IsInf(p.InputPerToken, 0) &&
		!math.IsInf(p.OutputPerToken, 0) &&
		!math.IsInf(p.CacheReadPerToken, 0) &&
		!math.IsInf(p.CacheWritePerToken, 0)
}

// ValidCost reports whether a computed cost is finite and non-negative.
// This catches float64 overflow from large token counts multiplied by
// large (but finite) prices, which can produce +Inf. Invalid costs
// are treated like unknown pricing: no cost attribute is emitted.
func ValidCost(cost float64) bool {
	return cost >= 0 &&
		!math.IsNaN(cost) &&
		!math.IsInf(cost, 0)
}
