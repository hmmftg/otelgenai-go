package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/conformancetest"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// newTestInstrumenter creates an Instrumenter backed by an in-memory
// span exporter for testing.
func newTestInstrumenter(t *testing.T) (*otelgenai.Instrumenter, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(tp),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return instr, exporter
}

func TestInference_Success(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	ctx, op := instr.StartInference(ctx, req)
	if op == nil {
		t.Fatal("StartInference returned nil op")
	}
	op.End(otelgenai.Response{
		Model:         "gpt-4o",
		ID:            "chatcmpl-123",
		FinishReasons:  []string{"stop"},
		Usage:         otelgenai.Usage{InputTokens: 10, OutputTokens: 20},
	}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertSpanName(t, span, "chat gpt-4o")
	conformancetest.AssertAttr(t, span, semconv.AttrGenAISystem, strVal("openai"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIRequestModel, strVal("gpt-4o"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIResponseModel, strVal("gpt-4o"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIResponseID, strVal("chatcmpl-123"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIUsageInputTokens, int64Val(10))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIUsageOutputTokens, int64Val(20))
	conformancetest.AssertSpanStatus(t, span, codes.Ok)
}

func TestInference_Error(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	ctx, op := instr.StartInference(ctx, req)
	op.End(otelgenai.Response{}, errors.New("some secret error message with PII"))

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertSpanStatus(t, span, codes.Error)
	conformancetest.AssertAttr(t, span, semconv.AttrErrorType, strVal("unknown"))
	// Verify the raw error message is NOT in the span.
	conformancetest.AssertNoSentinel(t, span, []string{"some secret error message with PII"})
}

func TestInference_NopWhenMissingMetadata(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	// Missing Model.
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
	})
	if op != nil {
		t.Fatal("expected nil op when Model is missing")
	}
	if got := exporter.GetSpans().Snapshots(); len(got) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(got))
	}
}

func TestInference_Cancelled(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	ctx, op := instr.StartInference(ctx, req)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	op.End(otelgenai.Response{}, ctx.Err())

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertAttr(t, span, semconv.AttrErrorType, strVal("cancelled"))
}

func TestInference_IdempotentEnd(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil) // should be no-op

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestInference_Disabled(t *testing.T) {
	instr, err := otelgenai.New(otelgenai.Disabled())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	if op != nil {
		t.Fatal("expected nil op when disabled")
	}
}
