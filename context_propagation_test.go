package otelgenai_test

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/testutil"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TestContextPropagation_AgentInferenceHierarchy verifies that
// inference spans are children of the agent span.
func TestContextPropagation_AgentInferenceHierarchy(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gpt-4o",
	})

	_, inferOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Find agent and inference spans.
	var agentSpan, inferSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch s.Name() {
		case "invoke_agent test-agent":
			agentSpan = s
		case "chat gpt-4o":
			inferSpan = s
		}
	}

	if agentSpan == nil {
		t.Fatal("agent span not found")
	}
	if inferSpan == nil {
		t.Fatal("inference span not found")
	}

	// Inference span's parent should be the agent span.
	if inferSpan.Parent().SpanID() != agentSpan.SpanContext().SpanID() {
		t.Errorf("inference parent: got %q, want %q (agent)",
			inferSpan.Parent().SpanID(), agentSpan.SpanContext().SpanID())
	}
}

// TestContextPropagation_AgentToolHierarchy verifies that
// tool spans are children of the agent span.
func TestContextPropagation_AgentToolHierarchy(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gpt-4o",
	})

	_, toolOp := instr.StartTool(ctx, otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
	})
	toolOp.End(nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	var agentSpan, toolSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch s.Name() {
		case "invoke_agent test-agent":
			agentSpan = s
		case "execute_tool get_weather":
			toolSpan = s
		}
	}

	if agentSpan == nil {
		t.Fatal("agent span not found")
	}
	if toolSpan == nil {
		t.Fatal("tool span not found")
	}

	// Tool span's parent should be the agent span.
	if toolSpan.Parent().SpanID() != agentSpan.SpanContext().SpanID() {
		t.Errorf("tool parent: got %q, want %q (agent)",
			toolSpan.Parent().SpanID(), agentSpan.SpanContext().SpanID())
	}
}

// TestContextPropagation_AgentInternalHierarchy verifies that
// internal operation spans are children of the agent span.
func TestContextPropagation_AgentInternalHierarchy(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gpt-4o",
	})

	_, internalOp := instr.StartInternalOperation(ctx, "custom_operation")
	internalOp.End(nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	var agentSpan, internalSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch s.Name() {
		case "invoke_agent test-agent":
			agentSpan = s
		case "custom_operation":
			internalSpan = s
		}
	}

	if agentSpan == nil {
		t.Fatal("agent span not found")
	}
	if internalSpan == nil {
		t.Fatal("internal span not found")
	}

	// Internal span's parent should be the agent span.
	if internalSpan.Parent().SpanID() != agentSpan.SpanContext().SpanID() {
		t.Errorf("internal parent: got %q, want %q (agent)",
			internalSpan.Parent().SpanID(), agentSpan.SpanContext().SpanID())
	}
}

// TestContextPropagation_NestedAgentInferenceTool verifies the full
// hierarchy: Agent → Inference → Tool.
func TestContextPropagation_NestedAgentInferenceTool(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gpt-4o",
	})

	_, inferOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, nil)

	_, toolOp := instr.StartTool(ctx, otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
	})
	toolOp.End(nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Verify hierarchy: agent is root, inference and tool are children of agent.
	var agentSpan, inferSpan, toolSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch s.Name() {
		case "invoke_agent test-agent":
			agentSpan = s
		case "chat gpt-4o":
			inferSpan = s
		case "execute_tool get_weather":
			toolSpan = s
		}
	}

	if agentSpan == nil || inferSpan == nil || toolSpan == nil {
		t.Fatal("missing expected spans")
	}

	// Both inference and tool should be children of the agent.
	if inferSpan.Parent().SpanID() != agentSpan.SpanContext().SpanID() {
		t.Errorf("inference parent: got %q, want %q",
			inferSpan.Parent().SpanID(), agentSpan.SpanContext().SpanID())
	}
	if toolSpan.Parent().SpanID() != agentSpan.SpanContext().SpanID() {
		t.Errorf("tool parent: got %q, want %q",
			toolSpan.Parent().SpanID(), agentSpan.SpanContext().SpanID())
	}

	// All spans should share the same trace.
	for _, s := range spans {
		if s.SpanContext().TraceID() != agentSpan.SpanContext().TraceID() {
			t.Errorf("trace ID mismatch for span %q", s.Name())
		}
	}
}

// TestContextPropagation_InferenceContextPropagated verifies that
// the context returned by StartInference contains the inference span.
func TestContextPropagation_InferenceContextPropagated(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, inferOp := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})

	// The context should contain a recording span.
	span := oteltrace.SpanFromContext(ctx)
	if !span.IsRecording() {
		t.Fatal("expected recording span in returned context")
	}

	inferOp.End(otelgenai.Response{}, nil)
}

// TestContextPropagation_ToolContextPropagated verifies that
// the context returned by StartTool contains the tool span.
func TestContextPropagation_ToolContextPropagated(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, toolOp := instr.StartTool(context.Background(), otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
	})

	span := oteltrace.SpanFromContext(ctx)
	if !span.IsRecording() {
		t.Fatal("expected recording span in returned context")
	}

	toolOp.End(nil)
}

// TestContextPropagation_InternalContextPropagated verifies that
// the context returned by StartInternalOperation contains the internal span.
func TestContextPropagation_InternalContextPropagated(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, internalOp := instr.StartInternalOperation(context.Background(), "test_op")

	span := oteltrace.SpanFromContext(ctx)
	if !span.IsRecording() {
		t.Fatal("expected recording span in returned context")
	}

	internalOp.End(nil)
}

// TestContextPropagation_AllSpansOKStatus verifies that successful
// operations across all types have OK span status.
func TestContextPropagation_AllSpansOKStatus(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gpt-4o",
	})

	_, inferOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	inferOp.End(otelgenai.Response{}, nil)

	_, toolOp := instr.StartTool(ctx, otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
	})
	toolOp.End(nil)

	_, internalOp := instr.StartInternalOperation(ctx, "custom_op")
	internalOp.End(nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 4 {
		t.Fatalf("expected 4 spans, got %d", len(spans))
	}

	for _, s := range spans {
		if s.Status().Code != codes.Ok {
			t.Errorf("span %q has non-OK status: %v", s.Name(), s.Status().Code)
		}
	}
}
