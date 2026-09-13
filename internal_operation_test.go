package otelgenai

import (
	"context"
	"errors"
	"testing"

	"github.com/hmmftg/otelgenai-go/testutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestStartInternalOperation_BasicSpan(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	customAttr := attribute.String("mcp.method.name", "resources/read")
	_, op := instr.StartInternalOperation(context.Background(), "resources/read", customAttr)
	if op == nil {
		t.Fatal("expected non-nil operation")
	}
	op.End(nil)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanName(t, s, "resources/read")
	testutil.AssertSpanKind(t, s, oteltrace.SpanKindInternal)
	testutil.AssertSpanStatus(t, s, codes.Ok)
	testutil.AssertAttr(t, s, "mcp.method.name", attribute.StringValue("resources/read"))
}

func TestStartInternalOperation_ErrorClassification(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, op := instr.StartInternalOperation(context.Background(), "prompts/get")
	if op == nil {
		t.Fatal("expected non-nil operation")
	}
	op.End(errors.New("some failure"))

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanStatus(t, s, codes.Error)
	testutil.AssertErrorType(t, s, string(ErrorTypeUnknown))
}

func TestStartInternalOperation_Disabled(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		Disabled(),
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, op := instr.StartInternalOperation(context.Background(), "resources/read")
	if op != nil {
		t.Fatal("expected nil operation when disabled")
	}
	// nil-safe End
	op.End(nil)
}

func TestStartInternalOperation_IdempotentEnd(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, op := instr.StartInternalOperation(context.Background(), "resources/read")
	op.End(nil)
	op.End(nil) // second call should be no-op
	op.End(errors.New("too late"))

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span (idempotent), got %d", len(spans))
	}
	testutil.AssertSpanStatus(t, spans[0], codes.Ok)
}

func TestStartInternalOperation_NilSafeEnd(t *testing.T) {
	var op *InternalOperation
	op.End(nil)             // should not panic
	op.End(errors.New("x")) // should not panic
}

func TestStartInternalOperation_NoAgentCounterIncrement(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Start an agent, then an internal operation inside it.
	_, agentOp := instr.StartAgent(context.Background(), AgentRequest{Name: "test-agent"})
	_, internalOp := instr.StartInternalOperation(context.Background(), "resources/read")
	internalOp.End(nil)
	agentOp.End(nil)

	// The internal operation should not increment tool_calls metric.
	// The agent always emits the metric, but the value should be 0.
	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.invoke_agent.tool_calls" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				continue
			}
			for _, dp := range hist.DataPoints {
				if dp.Sum != 0 {
					t.Errorf("internal operation should not increment tool_calls: got %d", dp.Sum)
				}
			}
		}
	}
}

func TestToolRequest_Attrs(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := New(
		WithTracerProvider(rec.TracerProvider()),
		WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	customAttrs := []attribute.KeyValue{
		attribute.String("mcp.method.name", "tools/call"),
	}
	_, op := instr.StartTool(context.Background(), ToolRequest{
		Name:  "get_weather",
		Type:  "mcp",
		Attrs: customAttrs,
	})
	op.End(nil)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertAttr(t, s, "mcp.method.name", attribute.StringValue("tools/call"))
	testutil.AssertAttr(t, s, "gen_ai.tool.name", attribute.StringValue("get_weather"))
	testutil.AssertAttr(t, s, "gen_ai.tool.type", attribute.StringValue("mcp"))
}
