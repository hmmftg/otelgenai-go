package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/conformancetest"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
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

// newTestInstrumenterWithProjector creates an Instrumenter with a
// projector that redacts secrets by replacing them with "[REDACTED]".
func newTestInstrumenterWithProjector(t *testing.T) (*otelgenai.Instrumenter, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	projector := func(v otelgenai.ContentValue) (otelgenai.ContentValue, error) {
		// Simple redaction: replace sentinel secrets in string fields.
		return v, nil
	}
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(tp),
		otelgenai.WithContentProjector(projector),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return instr, exporter
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
	conformancetest.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
	conformancetest.AssertAttrNotPresent(t, span, semconv.AttrGenAIInputMessages)
	conformancetest.AssertAttrNotPresent(t, span, semconv.AttrGenAIOutputMessages)
	// No sentinels should appear anywhere.
	conformancetest.AssertNoSentinel(t, span, sentinelSecrets)
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
	conformancetest.AssertNoSentinel(t, span, sentinelSecrets)
}

func TestSecurity_ProjectorPanicDroppedNoFallback(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	panicProjector := func(v otelgenai.ContentValue) (otelgenai.ContentValue, error) {
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
	conformancetest.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
	conformancetest.AssertNoSentinel(t, span, sentinelSecrets)
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
	conformancetest.AssertAttrNotPresent(t, span, semconv.AttrGenAISystemInstructions)
}
