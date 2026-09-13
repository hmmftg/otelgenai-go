package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// setupTestPropagator installs a W3C TraceContext + Baggage propagator
// and returns a cleanup function.
func setupTestPropagator() func() {
	orig := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return func() { otel.SetTextMapPropagator(orig) }
}

// noopNext is a MethodHandler that does nothing and returns nil.
func noopNext(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return nil, nil
}

// errorNext is a MethodHandler that returns an error.
func errorNext(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return nil, errors.New("tool failed")
}

func TestClientMiddleware_ToolsCall_Success(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanName(t, s, "execute_tool get_weather")
	testutil.AssertSpanKind(t, s, oteltrace.SpanKindInternal)
	testutil.AssertSpanStatus(t, s, codes.Ok)
	testutil.AssertAttr(t, s, AttrMCPMethodName, attribute.StringValue("tools/call"))
	testutil.AssertAttr(t, s, "gen_ai.tool.name", attribute.StringValue("get_weather"))
	testutil.AssertAttr(t, s, "gen_ai.tool.type", attribute.StringValue("mcp"))
}

func TestClientMiddleware_ToolsCall_Error(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(errorNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanStatus(t, s, codes.Error)
	testutil.AssertErrorType(t, s, string(otelgenai.ErrorTypeUnknown))
}

func TestClientMiddleware_ResourcesRead_Success(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ClientRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{URI: "file:///test.txt"},
	}
	_, _ = handler(context.Background(), MethodResourcesRead, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanName(t, s, "resources/read")
	testutil.AssertSpanKind(t, s, oteltrace.SpanKindInternal)
	testutil.AssertSpanStatus(t, s, codes.Ok)
	testutil.AssertAttr(t, s, AttrMCPMethodName, attribute.StringValue("resources/read"))
	testutil.AssertAttr(t, s, AttrMCPResourceURI, attribute.StringValue("file:///test.txt"))
	// URI should NOT be in span name.
	if s.Name() != "resources/read" {
		t.Errorf("span name should not contain URI: got %q", s.Name())
	}
}

func TestClientMiddleware_PromptsGet_Success(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ClientRequest[*mcp.GetPromptParams]{
		Params: &mcp.GetPromptParams{Name: "analyze-code"},
	}
	_, _ = handler(context.Background(), MethodPromptsGet, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanName(t, s, "prompts/get")
	testutil.AssertSpanKind(t, s, oteltrace.SpanKindInternal)
	testutil.AssertSpanStatus(t, s, codes.Ok)
	testutil.AssertAttr(t, s, AttrMCPMethodName, attribute.StringValue("prompts/get"))
	testutil.AssertAttr(t, s, AttrGenAIPromptName, attribute.StringValue("analyze-code"))
	// Name should NOT be in span name.
	if s.Name() != "prompts/get" {
		t.Errorf("span name should not contain prompt name: got %q", s.Name())
	}
}

func TestClientMiddleware_NonInstrumentedMethod_NoSpan(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	// Use a non-instrumented method like "ping".
	req := &mcp.ClientRequest[*mcp.PingParams]{
		Params: &mcp.PingParams{},
	}
	_, _ = handler(context.Background(), "ping", req)

	spans := rec.Spans()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans for non-instrumented method, got %d", len(spans))
	}
}

func TestClientMiddleware_NonInstrumentedMethod_NoMetaMutation(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	originalMeta := mcp.Meta{"existing": "value"}
	req := &mcp.ClientRequest[*mcp.PingParams]{
		Params: &mcp.PingParams{Meta: originalMeta},
	}
	_, _ = handler(context.Background(), "ping", req)

	// Verify _meta was not mutated.
	if _, ok := req.Params.GetMeta()["traceparent"]; ok {
		t.Error("non-instrumented method should not have traceparent injected")
	}
	if v, ok := req.Params.GetMeta()["existing"]; !ok || v != "value" {
		t.Error("non-instrumented method _meta was mutated")
	}
}

func TestClientMiddleware_DisabledInstrumenter_NoSpan(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.Disabled(),
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	spans := rec.Spans()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans when disabled, got %d", len(spans))
	}
}

func TestClientMiddleware_MetaTraceparentInjected(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	// Verify traceparent was injected into _meta.
	meta := req.Params.GetMeta()
	if meta == nil {
		t.Fatal("expected non-nil _meta after middleware")
	}
	if _, ok := meta["traceparent"]; !ok {
		t.Error("traceparent not found in _meta after client middleware")
	}
}

func TestClientMiddleware_MetaOriginalNotMutated(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	originalMeta := mcp.Meta{"existing": "value"}
	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather", Meta: originalMeta},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	// Original map should not be mutated.
	if _, ok := originalMeta["traceparent"]; ok {
		t.Error("original _meta map was mutated")
	}
	if len(originalMeta) != 1 {
		t.Errorf("original map size changed: got %d, want 1", len(originalMeta))
	}

	// Wire _meta should have existing + traceparent.
	meta := req.Params.GetMeta()
	if v, ok := meta["existing"]; !ok || v != "value" {
		t.Error("existing _meta value not preserved in wire meta")
	}
	if _, ok := meta["traceparent"]; !ok {
		t.Error("traceparent not in wire _meta")
	}
}

func TestClientMiddleware_AgentHierarchy_ToolCallsIncrement(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	// Start an agent context.
	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{Name: "test-agent"})
	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = handler(ctx, MethodToolsCall, req)
	agentOp.End(nil)

	// Verify tool_calls metric is 1.
	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.invoke_agent.tool_calls" {
				continue
			}
			// Check value is 1.
			if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
				for _, dp := range hist.DataPoints {
					if dp.Sum != 1 {
						t.Errorf("expected tool_calls=1, got %d", dp.Sum)
					}
				}
			}
		}
	}
}

func TestClientMiddleware_AgentHierarchy_ResourcesReadNoIncrement(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mw := ClientMiddleware(instr)
	handler := mw(noopNext)

	// Start an agent context.
	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{Name: "test-agent"})
	req := &mcp.ClientRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{URI: "file:///test.txt"},
	}
	_, _ = handler(ctx, MethodResourcesRead, req)
	agentOp.End(nil)

	// Verify tool_calls metric is 0.
	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.invoke_agent.tool_calls" {
				continue
			}
			if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
				for _, dp := range hist.DataPoints {
					if dp.Sum != 0 {
						t.Errorf("expected tool_calls=0 for resources/read, got %d", dp.Sum)
					}
				}
			}
		}
	}
}
