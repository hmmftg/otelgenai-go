package otelgenai_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/pricing"
	"github.com/hmmftg/otelgenai-go/testutil"
)

// newPricingInstrumenter creates an Instrumenter with a pricing resolver.
func newPricingInstrumenter(t *testing.T, resolver pricing.PricingResolver) (*otelgenai.Instrumenter, *tracetestExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	opts := []otelgenai.Option{
		otelgenai.WithTracerProvider(tp),
	}
	if resolver != nil {
		opts = append(opts, otelgenai.WithPricingResolver(resolver))
	}
	instr, err := otelgenai.New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return instr, &tracetestExporter{exporter}
}

type tracetestExporter struct {
	inner *tracetest.InMemoryExporter
}

func (e *tracetestExporter) Spans() []sdktrace.ReadOnlySpan {
	return e.inner.GetSpans().Snapshots()
}

func TestCostEstimation_WithResolver(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {
			InputPerToken:  0.00001,
			OutputPerToken: 0.00003,
		},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// 1000*0.00001 + 500*0.00003 = 0.01 + 0.015 = 0.025
	testutil.AssertEstimatedCost(t, spans[0], 0.025)
}

func TestCostEstimation_NoResolver(t *testing.T) {
	instr, exp := newPricingInstrumenter(t, nil)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_UnknownModel(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_ZeroTokens(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: 0.01},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_ResolverPanic(t *testing.T) {
	resolver := &panickingResolver{}
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// Panic should be caught; no cost attribute; inference succeeds.
	testutil.AssertNoEstimatedCost(t, spans[0])
}

type panickingResolver struct{}

func (p *panickingResolver) Resolve(key pricing.ModelPricingKey) (pricing.Price, bool) {
	panic("resolver panic")
}

func TestCostEstimation_CacheTokensSubset(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {
			InputPerToken:      0.00001,
			OutputPerToken:     0.00003,
			CacheReadPerToken:  0.000001,
			CacheWritePerToken: 0.000002,
		},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{
			InputTokens:      1000,
			OutputTokens:     200,
			CacheReadTokens:  400,
			CacheWriteTokens: 100,
		},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// nonCached = 1000 - 400 - 100 = 500
	// 500*0.00001 + 200*0.00003 + 400*0.000001 + 100*0.000002
	// = 0.005 + 0.006 + 0.0004 + 0.0002 = 0.0116
	testutil.AssertEstimatedCost(t, spans[0], 0.0116)
}

func TestCostEstimation_InconsistentUsage(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: 0.01},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{
			InputTokens:      1000,
			CacheReadTokens:  600,
			CacheWriteTokens: 500, // 600 + 500 > 1000
		},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_ErrorWithUsage(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {
			InputPerToken:  0.00001,
			OutputPerToken: 0.00003,
		},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	// Inference returns an error AND response contains valid usage.
	// Estimated cost should still be recorded, error.type should be
	// recorded, and the original error is returned unchanged.
	originalErr := errors.New("provider error")
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, originalErr)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Cost should be recorded despite the error.
	// 1000*0.00001 + 500*0.00003 = 0.025
	testutil.AssertEstimatedCost(t, span, 0.025)
	// Error type should also be recorded.
	testutil.AssertErrorType(t, span, "unknown")
}

func TestCostEstimation_NegativePrice(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: -0.01},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_NaNPrice(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: math.NaN()},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_InfPrice(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: math.Inf(1)},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_FreeModel(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {}, // all zero = free
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 1000, OutputTokens: 500},
	}, nil)

	spans := exp.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// Free model: cost == 0, no attribute emitted.
	testutil.AssertNoEstimatedCost(t, spans[0])
}

func TestCostEstimation_ConcurrentResolverCalls(t *testing.T) {
	resolver := pricing.NewStaticResolver(map[pricing.ModelPricingKey]pricing.Price{
		{System: "openai", Model: "gpt-4o"}: {InputPerToken: 0.00001},
	})
	instr, exp := newPricingInstrumenter(t, resolver)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			req := otelgenai.Request{
				Operation: otelgenai.Operation(semconv.OperationChat),
				Provider:  "openai",
				Model:     "gpt-4o",
			}
			_, op := instr.StartInference(ctx, req)
			op.End(otelgenai.Response{
				Model: "gpt-4o",
				Usage: otelgenai.Usage{InputTokens: 100, OutputTokens: 50},
			}, nil)
		}()
	}
	wg.Wait()

	spans := exp.Spans()
	if len(spans) != 50 {
		t.Fatalf("expected 50 spans, got %d", len(spans))
	}
	// All spans should have the cost attribute.
	for _, span := range spans {
		testutil.AssertEstimatedCost(t, span, 0.001) // 100*0.00001 + 50*0 = 0.001
	}
}
