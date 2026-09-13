package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestServerMiddleware_ToolsCall_Success(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
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

func TestServerMiddleware_ToolsCall_Error(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(errorNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
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

func TestServerMiddleware_ResourcesRead_Success(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ServerRequest[*mcp.ReadResourceParams]{
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
	testutil.AssertAttr(t, s, AttrMCPMethodName, attribute.StringValue("resources/read"))
	testutil.AssertAttr(t, s, AttrMCPResourceURI, attribute.StringValue("file:///test.txt"))
}

func TestServerMiddleware_PromptsGet_Success(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ServerRequest[*mcp.GetPromptParams]{
		Params: &mcp.GetPromptParams{Name: "analyze-code"},
	}
	_, _ = handler(context.Background(), MethodPromptsGet, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	testutil.AssertSpanName(t, s, "prompts/get")
	testutil.AssertAttr(t, s, AttrMCPMethodName, attribute.StringValue("prompts/get"))
	testutil.AssertAttr(t, s, AttrGenAIPromptName, attribute.StringValue("analyze-code"))
}

func TestServerMiddleware_NonInstrumentedMethod_NoSpan(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ServerRequest[*mcp.PingParams]{
		Params: &mcp.PingParams{},
	}
	_, _ = handler(context.Background(), "ping", req)

	spans := rec.Spans()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans for non-instrumented method, got %d", len(spans))
	}
}

func TestServerMiddleware_DisabledInstrumenter_NoSpan(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)

	spans := rec.Spans()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans when disabled, got %d", len(spans))
	}
}

func TestServerMiddleware_AgentCounterIsolation(t *testing.T) {
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

	mw := ServerMiddleware(instr)
	handler := mw(noopNext)

	// Start an agent context (simulating a local agent in the server process).
	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{Name: "server-agent"})
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
	}
	_, _ = handler(ctx, MethodToolsCall, req)
	agentOp.End(nil)

	// The server-side tools/call should NOT increment the local agent's
	// tool_calls counter because the server middleware extracts a fresh
	// context from _meta (which has no agentObserver).
	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.invoke_agent.tool_calls" {
				continue
			}
			if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
				for _, dp := range hist.DataPoints {
					if dp.Sum != 0 {
						t.Errorf("server-side tools/call should not increment local agent counter: got %d", dp.Sum)
					}
				}
			}
		}
	}
}

func TestServerMiddleware_RemoteParentExtraction(t *testing.T) {
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

	// Create a client span and inject its context into _meta.
	tracer := rec.TracerProvider().Tracer("test")
	clientCtx, clientSpan := tracer.Start(context.Background(), "client-span")

	meta := InjectTraceContext(clientCtx, mcp.Meta{})

	// Now run the server middleware with the injected _meta.
	serverMW := ServerMiddleware(instr)
	handler := serverMW(noopNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Name: "get_weather",
			Meta: meta,
		},
	}
	_, _ = handler(context.Background(), MethodToolsCall, req)
	clientSpan.End()

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (client + server), got %d", len(spans))
	}

	// Find the server span (execute_tool).
	var serverSpan, clientSpanFound = spans[0], spans[1]
	if serverSpan.Name() != "execute_tool get_weather" {
		serverSpan, clientSpanFound = clientSpanFound, serverSpan
	}

	// Verify the server span's parent is the client span.
	if serverSpan.Parent().SpanID() != clientSpanFound.SpanContext().SpanID() {
		t.Errorf("server span parent mismatch: got %q, want %q",
			serverSpan.Parent().SpanID(), clientSpanFound.SpanContext().SpanID())
	}
}

// TestServerMiddleware_PreservesCancellation verifies that the server
// middleware preserves cancellation from the incoming SDK request context.
func TestServerMiddleware_PreservesCancellation(t *testing.T) {
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

	// Create a cancellable context simulating the SDK request context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// The handler should observe the cancelled context.
	handlerObservedCancel := false
	cancelNext := func(handlerCtx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		if handlerCtx.Err() == context.Canceled {
			handlerObservedCancel = true
		}
		return nil, nil
	}

	mw := ServerMiddleware(instr)
	handler := mw(cancelNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
	}
	_, _ = handler(ctx, MethodToolsCall, req)

	if !handlerObservedCancel {
		t.Error("server handler did not observe cancellation from incoming context")
	}
}

// TestServerMiddleware_PreservesDeadline verifies that the server
// middleware preserves deadlines from the incoming SDK request context.
func TestServerMiddleware_PreservesDeadline(t *testing.T) {
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

	// Create a context with a deadline simulating the SDK request context.
	deadline := time.Now().Add(50 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	// The handler should observe the deadline.
	handlerObservedDeadline := false
	deadlineNext := func(handlerCtx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		if dl, ok := handlerCtx.Deadline(); ok && dl.Equal(deadline) {
			handlerObservedDeadline = true
		}
		return nil, nil
	}

	mw := ServerMiddleware(instr)
	handler := mw(deadlineNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
	}
	_, _ = handler(ctx, MethodToolsCall, req)

	if !handlerObservedDeadline {
		t.Error("server handler did not observe deadline from incoming context")
	}
}

// TestServerMiddleware_PreservesContextValues verifies that the server
// middleware preserves arbitrary context values from the incoming
// SDK request context (while still stripping the agent observer).
func TestServerMiddleware_PreservesContextValues(t *testing.T) {
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

	type testKey struct{}
	const testValue = "request-scoped-value"
	ctx := context.WithValue(context.Background(), testKey{}, testValue)

	// The handler should observe the context value.
	handlerObservedValue := false
	valueNext := func(handlerCtx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		if v, ok := handlerCtx.Value(testKey{}).(string); ok && v == testValue {
			handlerObservedValue = true
		}
		return nil, nil
	}

	mw := ServerMiddleware(instr)
	handler := mw(valueNext)

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "get_weather"},
	}
	_, _ = handler(ctx, MethodToolsCall, req)

	if !handlerObservedValue {
		t.Error("server handler did not observe context value from incoming context")
	}
}
