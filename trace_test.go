package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
)

func TestTraceInference_Success(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	resp, err := otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		return otelgenai.Response{
			Model:         "gpt-4o",
			ID:            "chatcmpl-123",
			FinishReasons: []string{"stop"},
			Usage:         otelgenai.Usage{InputTokens: 10, OutputTokens: 20},
		}, nil
	})
	if err != nil {
		t.Fatalf("TraceInference err: %v", err)
	}
	if resp.ID != "chatcmpl-123" {
		t.Fatalf("TraceInference resp.ID: got %q, want chatcmpl-123", resp.ID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertSpanName(t, span, "chat gpt-4o")
	testutil.AssertAttr(t, span, semconv.AttrGenAIResponseID, strVal("chatcmpl-123"))
	testutil.AssertAttr(t, span, semconv.AttrGenAIUsageInputTokens, int64Val(10))
	testutil.AssertAttr(t, span, semconv.AttrGenAIUsageOutputTokens, int64Val(20))
}

func TestTraceInference_Error(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	sentinelErr := errors.New("some secret error message with PII")
	resp, err := otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		return otelgenai.Response{}, sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("TraceInference err: got %v, want %v", err, sentinelErr)
	}
	if resp.ID != "" {
		t.Fatalf("TraceInference resp.ID: got %q, want empty", resp.ID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertSpanStatus(t, span, codes.Error)
	testutil.AssertAttr(t, span, semconv.AttrErrorType, strVal("unknown"))
	testutil.AssertNoSentinel(t, span, []string{"some secret error message with PII"})
}

func TestTraceInference_Disabled(t *testing.T) {
	instr, err := otelgenai.New(otelgenai.Disabled())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	resp, err := otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		return otelgenai.Response{Model: "gpt-4o"}, nil
	})
	if err != nil {
		t.Fatalf("TraceInference err: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Fatalf("TraceInference resp.Model: got %q, want gpt-4o", resp.Model)
	}
}

func TestTraceInference_NopWhenMissingMetadata(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	// Missing Model.
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
	}
	resp, err := otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		return otelgenai.Response{Model: "gpt-4o"}, nil
	})
	if err != nil {
		t.Fatalf("TraceInference err: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Fatalf("TraceInference resp.Model: got %q, want gpt-4o", resp.Model)
	}
	if got := exporter.GetSpans().Snapshots(); len(got) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(got))
	}
}

func TestTraceInference_ChildContextPropagated(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	// Verify the callback receives a context that is the child of the
	// inference span (i.e. the span is in the context).
	_, err := otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		// The inference span should be the active span in ctx.
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			t.Fatal("expected recording span in callback context")
		}
		return otelgenai.Response{Model: "gpt-4o"}, nil
	})
	if err != nil {
		t.Fatalf("TraceInference err: %v", err)
	}
	if got := exporter.GetSpans().Snapshots(); len(got) != 1 {
		t.Fatalf("expected 1 span, got %d", len(got))
	}
}

func TestTraceInference_PanicPropagates(t *testing.T) {
	instr, _ := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}
		// A panic may leave the span un-ended; we only assert the panic
		// propagated. The span may or may not be flushed depending on
		// where the panic occurred relative to op.End.
	}()
	_, _ = otelgenai.TraceInference(ctx, instr, req, func(ctx context.Context) (otelgenai.Response, error) {
		panic("callback panic")
	})
}

func TestTraceAgent_Success(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.AgentRequest{
		Name:        "my-agent",
		Description: "test agent",
		Model:       "gpt-4o",
	}
	result, err := otelgenai.TraceAgent(ctx, instr, req, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil || result != 42 {
		t.Fatalf("TraceAgent: result=%d, err=%v", result, err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertSpanName(t, span, "invoke_agent my-agent")
	testutil.AssertAttr(t, span, semconv.AttrGenAIAgentName, strVal("my-agent"))
}

func TestTraceAgent_Error(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.AgentRequest{Name: "agent"}
	sentinelErr := errors.New("agent failed")
	result, err := otelgenai.TraceAgent(ctx, instr, req, func(ctx context.Context) (int, error) {
		return 0, sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("TraceAgent err: got %v, want %v", err, sentinelErr)
	}
	if result != 0 {
		t.Fatalf("TraceAgent result: got %d, want 0", result)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	testutil.AssertAttr(t, span, semconv.AttrErrorType, strVal("unknown"))
}

func TestTraceAgent_Disabled(t *testing.T) {
	instr, err := otelgenai.New(otelgenai.Disabled())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	req := otelgenai.AgentRequest{Name: "agent"}
	result, err := otelgenai.TraceAgent(ctx, instr, req, func(ctx context.Context) (int, error) {
		return 7, nil
	})
	if err != nil || result != 7 {
		t.Fatalf("TraceAgent: result=%d, err=%v", result, err)
	}
}

func TestTraceAgent_NestedInferenceCounted(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.AgentRequest{Name: "parent"}
	_, err := otelgenai.TraceAgent(ctx, instr, req, func(ctx context.Context) (int, error) {
		// Start an inference call inside the agent.
		_, infOp := instr.StartInference(ctx, otelgenai.Request{
			Operation: otelgenai.Operation(semconv.OperationChat),
			Provider:  "openai",
			Model:     "gpt-4o",
		})
		infOp.End(otelgenai.Response{Model: "gpt-4o"}, nil)
		return 1, nil
	})
	if err != nil {
		t.Fatalf("TraceAgent err: %v", err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (agent + inference), got %d", len(spans))
	}
}

func TestTraceAgent_PanicPropagates(t *testing.T) {
	instr, _ := newTestInstrumenter(t)

	ctx := context.Background()
	req := otelgenai.AgentRequest{Name: "agent"}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}
	}()
	_, _ = otelgenai.TraceAgent(ctx, instr, req, func(ctx context.Context) (int, error) {
		panic("callback panic")
	})
}
