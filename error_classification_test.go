package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
	"go.opentelemetry.io/otel/attribute"
)

// TestErrorClassification_Consistency verifies that the same error
// types are classified consistently across Inference, Tool, and
// InternalOperation spans.
func TestErrorClassification_Consistency(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantType otelgenai.ErrorType
	}{
		{"nil", nil, otelgenai.ErrorTypeNone},
		{"cancelled", context.Canceled, otelgenai.ErrorTypeCancelled},
		{"deadline", context.DeadlineExceeded, otelgenai.ErrorTypeTimeout},
		{"unknown", errors.New("something broke"), otelgenai.ErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := testutil.NewRecorder()
			defer rec.Shutdown(context.Background())

			instr, err := otelgenai.New(
				otelgenai.WithTracerProvider(rec.TracerProvider()),
				otelgenai.WithMeterProvider(rec.MeterProvider()),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			// Inference operation.
			_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
				Operation: otelgenai.Operation(semconv.OperationChat),
				Provider:  "openai",
				Model:     "gpt-4o",
			})
			inferOp.End(otelgenai.Response{}, tt.err)

			// Tool operation.
			_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
				Name: "test-tool",
				Type: "function",
			})
			toolOp.End(tt.err)

			// Internal operation.
			_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
			internalOp.End(tt.err)

			spans := rec.Spans()
			if len(spans) != 3 {
				t.Fatalf("expected 3 spans, got %d", len(spans))
			}

			for _, s := range spans {
				if tt.err == nil {
					// No error type attribute expected.
					testutil.AssertAttrNotPresent(t, s, semconv.AttrErrorType)
				} else {
					testutil.AssertAttr(t, s, semconv.AttrErrorType, attribute.StringValue(string(tt.wantType)))
				}
			}
		})
	}
}

// TestErrorClassification_CancelledContext verifies that context.Canceled
// is classified as "cancelled" across all operation types.
func TestErrorClassification_CancelledContext(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Inference.
	_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, context.Canceled)

	// Tool.
	_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "test-tool",
		Type: "function",
	})
	toolOp.End(context.Canceled)

	// Internal.
	_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
	internalOp.End(context.Canceled)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		testutil.AssertErrorType(t, s, string(otelgenai.ErrorTypeCancelled))
	}
}

// TestErrorClassification_DeadlineExceeded verifies that context.DeadlineExceeded
// is classified as "timeout" across all operation types.
func TestErrorClassification_DeadlineExceeded(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Inference.
	_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, context.DeadlineExceeded)

	// Tool.
	_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "test-tool",
		Type: "function",
	})
	toolOp.End(context.DeadlineExceeded)

	// Internal.
	_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
	internalOp.End(context.DeadlineExceeded)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		testutil.AssertErrorType(t, s, string(otelgenai.ErrorTypeTimeout))
	}
}

// TestErrorClassification_UnknownError verifies that unclassified errors
// are classified as "unknown" across all operation types.
func TestErrorClassification_UnknownError(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	customErr := errors.New("transport connection refused")

	// Inference.
	_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, customErr)

	// Tool.
	_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "test-tool",
		Type: "function",
	})
	toolOp.End(customErr)

	// Internal.
	_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
	internalOp.End(customErr)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		testutil.AssertErrorType(t, s, string(otelgenai.ErrorTypeUnknown))
	}
}

// TestErrorClassification_SuccessNoErrorType verifies that successful
// operations do not have an error.type attribute.
func TestErrorClassification_SuccessNoErrorType(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Inference.
	_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, nil)

	// Tool.
	_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "test-tool",
		Type: "function",
	})
	toolOp.End(nil)

	// Internal.
	_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
	internalOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		testutil.AssertAttrNotPresent(t, s, semconv.AttrErrorType)
	}
}

// TestErrorClassification_CustomClassifier verifies that a custom
// ErrorClassifier is used consistently across all operation types.
func TestErrorClassification_CustomClassifier(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	customClassifier := func(err error) otelgenai.ErrorType {
		if err == nil {
			return otelgenai.ErrorTypeNone
		}
		return otelgenai.ErrorTypeTransport
	}

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithErrorClassifier(customClassifier),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	customErr := errors.New("connection refused")

	// Inference.
	_, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, customErr)

	// Tool.
	_, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "test-tool",
		Type: "function",
	})
	toolOp.End(customErr)

	// Internal.
	_, internalOp := instr.StartInternalOperation(context.Background(), "test-internal")
	internalOp.End(customErr)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		testutil.AssertErrorType(t, s, string(otelgenai.ErrorTypeTransport))
	}
}
