package testutil_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
)

func TestRecorder_SpansAndSpanAccess(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{
		Model:         "gpt-4o",
		ID:            "chatcmpl-1",
		Usage:         otelgenai.Usage{InputTokens: 10, OutputTokens: 20},
		FinishReasons: []string{"stop"},
	}, nil)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if rec.Span(0) == nil {
		t.Fatal("Span(0) returned nil")
	}
	if rec.Span(1) != nil {
		t.Fatal("Span(1) should be nil for out-of-range")
	}

	span := rec.Span(0)
	testutil.AssertSpanName(t, span, "chat gpt-4o")
	testutil.AssertSpanKind(t, span, trace.SpanKindClient)
	testutil.AssertSpanStatus(t, span, codes.Ok)
	testutil.AssertAttr(t, span, semconv.AttrGenAISystem, attribute.StringValue("openai"))
	testutil.AssertAttr(t, span, semconv.AttrGenAIResponseID, attribute.StringValue("chatcmpl-1"))
}

func TestRecorder_AssertSpan(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	span := rec.Span(0)
	testutil.AssertSpan(t, span, "chat gpt-4o", trace.SpanKindClient, codes.Ok)
}

func TestRecorder_AssertNoContentCaptured(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation:          otelgenai.Operation(semconv.OperationChat),
		Provider:           "openai",
		Model:              "gpt-4o",
		SystemInstructions: "secret-instructions",
		InputMessages:      []otelgenai.Message{{Role: "user", Content: "secret-prompt"}},
	})
	op.End(otelgenai.Response{
		Model:          "gpt-4o",
		OutputMessages: []otelgenai.Message{{Role: "assistant", Content: "secret-response"}},
	}, nil)

	testutil.AssertNoContentCaptured(t, rec.Span(0))
}

func TestRecorder_AssertErrorType(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{}, errors.New("some error"))

	testutil.AssertErrorType(t, rec.Span(0), "unknown")
}

func TestRecorder_AssertStreaming(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	testutil.AssertStreaming(t, rec.Span(0), true)
}

func TestRecorder_AssertToolCall(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	)

	_, op := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
		ID:   "call_1",
	})
	op.End(nil)

	testutil.AssertToolCall(t, rec.Span(0), "get_weather", "function")
}

func TestRecorder_AssertTokenUsage(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{
		Model: "gpt-4o",
		Usage: otelgenai.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil)

	rm := testutil.CollectMetrics(t, rec)
	testutil.AssertTokenUsage(t, rm, "gpt-4o", 100, 50)
}

func TestRecorder_AssertAgentHierarchy(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)

	ctx := context.Background()
	ctx, agentOp := instr.StartAgent(ctx, otelgenai.AgentRequest{Name: "parent"})

	// One inference call inside the agent.
	_, infOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	infOp.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	// One tool call inside the agent.
	_, toolOp := instr.StartTool(ctx, otelgenai.ToolRequest{Name: "search"})
	toolOp.End(nil)

	agentOp.End(nil)

	spans := rec.Spans()
	testutil.AssertAgentHierarchy(t, spans, "parent", 1, 1)
}

func TestRecorder_Shutdown(t *testing.T) {
	rec := testutil.NewRecorder()
	if err := rec.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRecorder_FindSpan(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	)

	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	if s := testutil.FindSpan(rec.Spans(), "chat gpt-4o"); s == nil {
		t.Error("FindSpan returned nil for existing span")
	}
	if s := testutil.FindSpan(rec.Spans(), "nonexistent"); s != nil {
		t.Error("FindSpan returned non-nil for nonexistent span")
	}
}
