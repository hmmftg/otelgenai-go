package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
)

// sentinelSecrets are secret values that must never appear in
// telemetry. They are seeded into request content, response content,
// and error messages to verify no leakage.
var sentinelSecrets = []string{
	"SECRET_API_KEY_12345",
	"my-very-private-prompt-content",
	"response-with-sensitive-data",
	"tool-argument-secret",
}

func TestSecurity_NoContentByDefault(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation:          otelgenai.Operation(semconv.OperationChat),
		Provider:           "openai",
		Model:              "gpt-4o",
		SystemInstructions: "SECRET_API_KEY_12345",
		InputMessages:      []otelgenai.Message{{Role: "user", Content: "my-very-private-prompt-content"}},
	}
	_, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{
		Model:          "gpt-4o",
		OutputMessages: []otelgenai.Message{{Role: "assistant", Content: "response-with-sensitive-data"}},
	}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// No content attributes should be present by default.
	testutil.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
	testutil.AssertAttrNotPresent(t, span, semconv.AttrGenAIInputMessages)
	testutil.AssertAttrNotPresent(t, span, semconv.AttrGenAIOutputMessages)
	// No sentinels should appear anywhere.
	testutil.AssertNoSentinel(t, span, sentinelSecrets)
}

func TestSecurity_NoRawErrorInTelemetry(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	secretErr := errors.New("SECRET_API_KEY_12345 leaked in error")
	op.End(otelgenai.Response{}, secretErr)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertNoSentinel(t, span, sentinelSecrets)
}

func TestSecurity_ProjectorPanicDroppedNoFallback(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	panicProjector := func(_ otelgenai.ContentValue) (otelgenai.ContentValue, error) {
		panic("projector panic")
	}
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(tp),
		otelgenai.WithContentProjector(panicProjector),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation:          otelgenai.Operation(semconv.OperationChat),
		Provider:           "openai",
		Model:              "gpt-4o",
		SystemInstructions: "my-very-private-prompt-content",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// The projection should have been dropped; no content should appear.
	testutil.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
	testutil.AssertNoSentinel(t, span, sentinelSecrets)
}

func TestSecurity_ProjectorOversizeDropped(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	bigProjector := func(v otelgenai.ContentValue) (otelgenai.ContentValue, error) {
		// Return a value that exceeds the projection limit.
		return otelgenai.ContentValue{
			Kind:               v.Kind,
			SystemInstructions: string(make([]byte, 10000)),
		}, nil
	}
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(tp),
		otelgenai.WithContentProjector(bigProjector),
		otelgenai.WithProjectionLimit(100),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation:          otelgenai.Operation(semconv.OperationChat),
		Provider:           "openai",
		Model:              "gpt-4o",
		SystemInstructions: "test",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
}
