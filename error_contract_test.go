package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
)

// TestErrorContract_CrossAdapter verifies that the core error
// classification categories (cancelled, timeout, unknown) are
// consistent across all operation types. The test exercises the same
// core classification path rather than manufacturing provider-native
// errors that happen to look equivalent.
func TestErrorContract_CrossAdapter(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantType string
	}{
		{"no error", nil, ""},
		{"cancelled", context.Canceled, "cancelled"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"unknown", errors.New("something broke"), "unknown"},
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
				Name: "search",
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

			for _, span := range spans {
				if tt.wantType == "" {
					testutil.AssertAttrNotPresent(t, span, "error.type")
				} else {
					testutil.AssertErrorType(t, span, tt.wantType)
				}
			}
		})
	}
}
