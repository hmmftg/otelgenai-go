package otelgenai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/conformancetest"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func TestAgent_Success(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	ctx, op := instr.StartAgent(ctx, otelgenai.AgentRequest{
		Name:        "my-agent",
		Description: "test agent",
		Model:       "gpt-4o",
	})
	if op == nil {
		t.Fatal("StartAgent returned nil")
	}
	op.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertSpanName(t, span, "invoke_agent my-agent")
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIAgentName, strVal("my-agent"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIAgentDescription, strVal("test agent"))
}

func TestAgent_NoName(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartAgent(ctx, otelgenai.AgentRequest{})
	op.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	conformancetest.AssertSpanName(t, spans[0], "invoke_agent")
}

func TestAgent_Error(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartAgent(ctx, otelgenai.AgentRequest{Name: "agent"})
	op.End(errors.New("agent failed"))

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertAttr(t, span, semconv.AttrErrorType, strVal("unknown"))
}

func TestAgent_NestedInferenceCounted(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	ctx, agentOp := instr.StartAgent(ctx, otelgenai.AgentRequest{Name: "parent"})

	// Start an inference call inside the agent.
	_, infOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	infOp.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	agentOp.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
}

func TestTool_Success(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartTool(ctx, otelgenai.ToolRequest{
		Name: "get_weather",
		Type: "function",
		ID:   "call_123",
	})
	op.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertSpanName(t, span, "execute_tool get_weather")
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIToolName, strVal("get_weather"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIToolType, strVal("function"))
	conformancetest.AssertAttr(t, span, semconv.AttrGenAIToolCallID, strVal("call_123"))
}

func TestTool_Error(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartTool(ctx, otelgenai.ToolRequest{Name: "failing_tool"})
	op.End(errors.New("tool error"))

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	conformancetest.AssertAttr(t, span, semconv.AttrErrorType, strVal("unknown"))
}

func TestTool_DefaultType(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartTool(ctx, otelgenai.ToolRequest{Name: "tool"})
	op.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	conformancetest.AssertAttr(t, spans[0], semconv.AttrGenAIToolType, strVal("function"))
}

func TestTraceTool_Generic(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	result, err := otelgenai.TraceTool(instr, ctx, otelgenai.ToolRequest{Name: "compute"}, func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil || result != 42 {
		t.Fatalf("TraceTool: result=%d, err=%v", result, err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	conformancetest.AssertSpanName(t, spans[0], "execute_tool compute")
}

func TestAgentTool_NestedCounting(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	ctx, agentOp := instr.StartAgent(ctx, otelgenai.AgentRequest{Name: "agent"})

	// Tool call inside agent.
	_, toolOp := instr.StartTool(ctx, otelgenai.ToolRequest{Name: "tool1"})
	toolOp.End(nil)

	// Inference call inside agent.
	_, infOp := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	infOp.End(otelgenai.Response{Model: "gpt-4o"}, nil)

	agentOp.End(nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}
}
