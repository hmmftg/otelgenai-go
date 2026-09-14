package pricing

import (
	"math"
	"sync"
	"testing"
)

func TestNormalize_BasicValid(t *testing.T) {
	tests := []struct {
		name                                 string
		input, output, cacheRead, cacheWrite int64
		wantNonCached                        int64
	}{
		{"input only", 1000, 500, 0, 0, 1000},
		{"output only", 0, 500, 0, 0, 0},
		{"cache read subset", 1000, 500, 400, 0, 600},
		{"cache write subset", 1000, 500, 0, 100, 900},
		{"both cache subsets", 1000, 500, 400, 100, 500},
		{"input == cacheRead + cacheWrite", 1000, 500, 400, 600, 0},
		{"zero tokens", 0, 0, 0, 0, 0},
		{"large token counts", 1_000_000_000, 2_000_000_000, 500_000_000, 300_000_000, 200_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Normalize(tt.input, tt.output, tt.cacheRead, tt.cacheWrite)
			if !ok {
				t.Fatalf("Normalize(%d, %d, %d, %d): expected ok, got false",
					tt.input, tt.output, tt.cacheRead, tt.cacheWrite)
			}
			if got.NonCachedInputTokens != tt.wantNonCached {
				t.Errorf("NonCachedInputTokens: got %d, want %d",
					got.NonCachedInputTokens, tt.wantNonCached)
			}
			if got.OutputTokens != tt.output {
				t.Errorf("OutputTokens: got %d, want %d", got.OutputTokens, tt.output)
			}
			if got.CacheReadTokens != tt.cacheRead {
				t.Errorf("CacheReadTokens: got %d, want %d", got.CacheReadTokens, tt.cacheRead)
			}
			if got.CacheWriteTokens != tt.cacheWrite {
				t.Errorf("CacheWriteTokens: got %d, want %d", got.CacheWriteTokens, tt.cacheWrite)
			}
			// Invariant: nonCached + cacheRead + cacheWrite == input
			if got.NonCachedInputTokens+got.CacheReadTokens+got.CacheWriteTokens != tt.input {
				t.Errorf("invariant broken: %d + %d + %d != %d",
					got.NonCachedInputTokens, got.CacheReadTokens, got.CacheWriteTokens, tt.input)
			}
		})
	}
}

func TestNormalize_Invalid(t *testing.T) {
	tests := []struct {
		name                                 string
		input, output, cacheRead, cacheWrite int64
	}{
		{"cacheRead > input", 100, 50, 150, 0},
		{"cacheWrite > remaining", 1000, 50, 400, 700},
		{"subsets exceed aggregate", 1000, 50, 700, 400},
		{"negative input", -1, 0, 0, 0},
		{"negative output", 0, -1, 0, 0},
		{"negative cacheRead", 0, 0, -1, 0},
		{"negative cacheWrite", 0, 0, 0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := Normalize(tt.input, tt.output, tt.cacheRead, tt.cacheWrite)
			if ok {
				t.Fatalf("Normalize(%d, %d, %d, %d): expected false, got ok",
					tt.input, tt.output, tt.cacheRead, tt.cacheWrite)
			}
		})
	}
}

func TestNormalize_MaxIntOverflow(t *testing.T) {
	// cacheRead=MaxInt64, cacheWrite=1 must not wrap negative via
	// cacheRead + cacheWrite.
	_, ok := Normalize(0, 0, math.MaxInt64, 1)
	if ok {
		t.Fatalf("Normalize with cacheRead=MaxInt64, cacheWrite=1, input=0: expected false (cacheRead > input)")
	}
}

func TestNormalize_SubtractionAtLimit(t *testing.T) {
	// input=MaxInt64, cacheRead=MaxInt64, cacheWrite=0 → valid, nonCached=0
	got, ok := Normalize(math.MaxInt64, 0, math.MaxInt64, 0)
	if !ok {
		t.Fatalf("Normalize(MaxInt64, 0, MaxInt64, 0): expected ok")
	}
	if got.NonCachedInputTokens != 0 {
		t.Errorf("NonCachedInputTokens: got %d, want 0", got.NonCachedInputTokens)
	}

	// input=MaxInt64, cacheRead=MaxInt64-1, cacheWrite=1 → valid, nonCached=0
	got, ok = Normalize(math.MaxInt64, 0, math.MaxInt64-1, 1)
	if !ok {
		t.Fatalf("Normalize(MaxInt64, 0, MaxInt64-1, 1): expected ok")
	}
	if got.NonCachedInputTokens != 0 {
		t.Errorf("NonCachedInputTokens: got %d, want 0", got.NonCachedInputTokens)
	}
}

func TestEstimate(t *testing.T) {
	price := Price{
		InputPerToken:      0.00001,
		OutputPerToken:     0.00003,
		CacheReadPerToken:  0.000001,
		CacheWritePerToken: 0.000002,
	}
	usage := PriceableUsage{
		NonCachedInputTokens: 500,
		OutputTokens:         200,
		CacheReadTokens:      400,
		CacheWriteTokens:     100,
	}
	// 500*0.00001 + 200*0.00003 + 400*0.000001 + 100*0.000002
	// = 0.005 + 0.006 + 0.0004 + 0.0002 = 0.0116
	want := 0.0116
	got := Estimate(usage, price)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Estimate: got %v, want %v", got, want)
	}
}

func TestEstimate_ZeroPrice(t *testing.T) {
	price := Price{} // all zero = free
	usage := PriceableUsage{
		NonCachedInputTokens: 1000,
		OutputTokens:         500,
	}
	got := Estimate(usage, price)
	if got != 0 {
		t.Errorf("Estimate with zero price: got %v, want 0", got)
	}
}

func TestValidatePrice(t *testing.T) {
	tests := []struct {
		name string
		p    Price
		want bool
	}{
		{"valid", Price{InputPerToken: 0.01, OutputPerToken: 0.03}, true},
		{"zero (free)", Price{}, true},
		{"negative input", Price{InputPerToken: -0.01}, false},
		{"negative output", Price{OutputPerToken: -0.01}, false},
		{"negative cacheRead", Price{CacheReadPerToken: -0.01}, false},
		{"negative cacheWrite", Price{CacheWritePerToken: -0.01}, false},
		{"NaN input", Price{InputPerToken: math.NaN()}, false},
		{"NaN output", Price{OutputPerToken: math.NaN()}, false},
		{"NaN cacheRead", Price{CacheReadPerToken: math.NaN()}, false},
		{"NaN cacheWrite", Price{CacheWritePerToken: math.NaN()}, false},
		{"+Inf input", Price{InputPerToken: math.Inf(1)}, false},
		{"+Inf output", Price{OutputPerToken: math.Inf(1)}, false},
		{"-Inf cacheRead", Price{CacheReadPerToken: math.Inf(-1)}, false},
		{"+Inf cacheWrite", Price{CacheWritePerToken: math.Inf(1)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidatePrice(tt.p)
			if got != tt.want {
				t.Errorf("ValidatePrice: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidCost(t *testing.T) {
	tests := []struct {
		name string
		cost float64
		want bool
	}{
		{"zero", 0, true},
		{"positive", 0.0116, true},
		{"negative", -0.01, false},
		{"NaN", math.NaN(), false},
		{"+Inf", math.Inf(1), false},
		{"-Inf", math.Inf(-1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidCost(tt.cost)
			if got != tt.want {
				t.Errorf("ValidCost(%v): got %v, want %v", tt.cost, got, tt.want)
			}
		})
	}
}

func TestEstimate_FinitePriceHugeUsage(t *testing.T) {
	// Large but finite price and usage should produce finite cost.
	price := Price{InputPerToken: 0.01}
	usage := PriceableUsage{NonCachedInputTokens: 1_000_000_000}
	cost := Estimate(usage, price)
	if !ValidCost(cost) {
		t.Errorf("Expected finite cost, got %v", cost)
	}
}

func TestEstimate_OverflowToInf(t *testing.T) {
	// Large finite price × large usage → +Inf.
	price := Price{InputPerToken: 1e300}
	usage := PriceableUsage{NonCachedInputTokens: math.MaxInt64}
	cost := Estimate(usage, price)
	if ValidCost(cost) {
		t.Errorf("Expected +Inf cost to be rejected by ValidCost, got %v", cost)
	}
}

func TestStaticResolver_Basic(t *testing.T) {
	key := ModelPricingKey{System: "openai", Model: "gpt-4"}
	price := Price{InputPerToken: 0.01, OutputPerToken: 0.03}
	r := NewStaticResolver(map[ModelPricingKey]Price{key: price})

	got, ok := r.Resolve(key)
	if !ok {
		t.Fatalf("Resolve: expected ok")
	}
	if got != price {
		t.Errorf("Resolve: got %v, want %v", got, price)
	}
}

func TestStaticResolver_UnknownModel(t *testing.T) {
	r := NewStaticResolver(map[ModelPricingKey]Price{})
	_, ok := r.Resolve(ModelPricingKey{System: "openai", Model: "gpt-4"})
	if ok {
		t.Fatalf("Resolve unknown model: expected false")
	}
}

func TestStaticResolver_CopiesMap(t *testing.T) {
	key := ModelPricingKey{System: "openai", Model: "gpt-4"}
	price := Price{InputPerToken: 0.01}
	prices := map[ModelPricingKey]Price{key: price}
	r := NewStaticResolver(prices)

	// Mutate caller's map.
	delete(prices, key)

	got, ok := r.Resolve(key)
	if !ok {
		t.Fatalf("Resolve after caller delete: expected ok (resolver should have copied)")
	}
	if got.InputPerToken != price.InputPerToken {
		t.Errorf("Resolve after caller delete: got %v, want %v", got.InputPerToken, price.InputPerToken)
	}
}

func TestStaticResolver_NilMap(t *testing.T) {
	r := NewStaticResolver(nil)
	_, ok := r.Resolve(ModelPricingKey{System: "openai", Model: "gpt-4"})
	if ok {
		t.Fatalf("Resolve with nil map: expected false")
	}
}

func TestStaticResolver_ConcurrentAccess(t *testing.T) {
	key := ModelPricingKey{System: "openai", Model: "gpt-4"}
	r := NewStaticResolver(map[ModelPricingKey]Price{key: {InputPerToken: 0.01}})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Resolve(key)
		}()
	}
	wg.Wait()
}

func TestCustomResolver(t *testing.T) {
	// A custom resolver that always returns a fixed price.
	custom := &customResolver{price: Price{InputPerToken: 0.02}, ok: true}
	got, ok := custom.Resolve(ModelPricingKey{System: "x", Model: "y"})
	if !ok {
		t.Fatalf("custom Resolve: expected ok")
	}
	if got.InputPerToken != 0.02 {
		t.Errorf("custom Resolve: got %v, want 0.02", got.InputPerToken)
	}
}

type customResolver struct {
	price Price
	ok    bool
}

func (c *customResolver) Resolve(key ModelPricingKey) (Price, bool) {
	return c.price, c.ok
}
