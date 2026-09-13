package mcp

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestClientSpanInjection verifies the critical ordering: the client
// middleware starts the operation FIRST, then injects the span-enriched
// context into _meta. The server span's remote parent must be the client
// span, not the caller's parent span.
func TestClientSpanInjection(t *testing.T) {
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

	// Create a caller span.
	tracer := rec.TracerProvider().Tracer("test")
	callerCtx, callerSpan := tracer.Start(context.Background(), "caller-span")

	// Run client middleware with the caller context.
	clientMW := ClientMiddleware(instr)
	clientHandler := clientMW(noopNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = clientHandler(callerCtx, MethodToolsCall, req)

	// Extract traceparent from _meta and verify it contains the client span's SpanID.
	meta := req.Params.GetMeta()
	if meta == nil {
		t.Fatal("expected non-nil _meta")
	}
	tp, ok := meta["traceparent"]
	if !ok {
		t.Fatal("traceparent not found in _meta")
	}
	tpStr, ok := tp.(string)
	if !ok {
		t.Fatalf("traceparent is not a string: %T", tp)
	}

	// Now run server middleware with the extracted _meta.
	serverMW := ServerMiddleware(instr)
	serverHandler := serverMW(noopNext)

	serverReq := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Name: "get_weather",
			Meta: mcp.Meta(meta),
		},
	}
	_, _ = serverHandler(context.Background(), MethodToolsCall, serverReq)
	callerSpan.End()

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans (caller + client + server), got %d", len(spans))
	}

	// Find spans by name.
	var callerS, clientS, serverS sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch s.Name() {
		case "caller-span":
			callerS = s
		case "execute_tool get_weather":
			// Could be client or server - check parent.
			if clientS == nil {
				clientS = s
			} else {
				serverS = s
			}
		}
	}

	if callerS == nil {
		t.Fatal("caller span not found")
	}
	if clientS == nil {
		t.Fatal("client span not found")
	}
	if serverS == nil {
		t.Fatal("server span not found")
	}

	// The client span's parent should be the caller span.
	if clientS.Parent().SpanID() != callerS.SpanContext().SpanID() {
		t.Errorf("client span parent: got %q, want %q (caller)",
			clientS.Parent().SpanID(), callerS.SpanContext().SpanID())
	}

	// The server span's parent should be the client span (not the caller).
	if serverS.Parent().SpanID() != clientS.SpanContext().SpanID() {
		t.Errorf("server span parent: got %q, want %q (client)",
			serverS.Parent().SpanID(), clientS.SpanContext().SpanID())
	}

	// Verify the traceparent in _meta contains the client span's SpanID.
	// traceparent format: 00-<traceID>-<spanID>-<flags>
	// The spanID field (chars 37-53) should be the client span's SpanID.
	if len(tpStr) < 53 {
		t.Fatalf("traceparent too short: %q", tpStr)
	}
	tpSpanID := tpStr[36:52]
	if tpSpanID != clientS.SpanContext().SpanID().String() {
		t.Errorf("traceparent span-id: got %q, want %q (client span)",
			tpSpanID, clientS.SpanContext().SpanID().String())
	}
}

// TestBothSidesInstrumented verifies that when both client and server
// middleware are enabled, both spans are created and the server span
// is a child of the client span via _meta trace-context propagation.
func TestBothSidesInstrumented(t *testing.T) {
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

	// Client middleware.
	clientMW := ClientMiddleware(instr)
	clientHandler := clientMW(noopNext)

	// Server middleware.
	serverMW := ServerMiddleware(instr)
	serverHandler := serverMW(noopNext)

	// Run client middleware.
	clientReq := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = clientHandler(context.Background(), MethodToolsCall, clientReq)

	// Extract _meta from client request and pass to server.
	meta := clientReq.Params.GetMeta()

	// Run server middleware with the extracted _meta.
	serverReq := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Name: "get_weather",
			Meta: mcp.Meta(meta),
		},
	}
	_, _ = serverHandler(context.Background(), MethodToolsCall, serverReq)

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (client + server), got %d", len(spans))
	}

	// Both spans should be named "execute_tool get_weather".
	// Identify client vs server by parent: client has no parent (root),
	// server has client as parent.
	var clientS, serverS sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Parent().SpanID() == [8]byte{} || s.Parent().SpanID().String() == "0000000000000000" {
			clientS = s
		} else {
			serverS = s
		}
	}

	if clientS == nil {
		t.Fatal("client span (root) not found")
	}
	if serverS == nil {
		t.Fatal("server span (child) not found")
	}

	// Server span's parent should be the client span.
	if serverS.Parent().SpanID() != clientS.SpanContext().SpanID() {
		t.Errorf("server span parent: got %q, want %q (client)",
			serverS.Parent().SpanID(), clientS.SpanContext().SpanID())
	}
}

// TestNoContentLeakage verifies that tool arguments, resource contents,
// and prompt messages are never captured in MCP spans.
func TestNoContentLeakage(t *testing.T) {
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

	// Test tools/call with arguments.
	clientMW := ClientMiddleware(instr)
	clientHandler := clientMW(noopNext)

	toolReq := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{
			Name:      "get_weather",
			Arguments: map[string]any{"location": "secret-location"},
		},
	}
	_, _ = clientHandler(context.Background(), MethodToolsCall, toolReq)

	// Test resources/read.
	resourceReq := &mcp.ClientRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{URI: "file:///secret.txt"},
	}
	_, _ = clientHandler(context.Background(), MethodResourcesRead, resourceReq)

	// Test prompts/get with arguments.
	promptReq := &mcp.ClientRequest[*mcp.GetPromptParams]{
		Params: &mcp.GetPromptParams{
			Name:      "analyze-code",
			Arguments: map[string]string{"code": "secret-code"},
		},
	}
	_, _ = clientHandler(context.Background(), MethodPromptsGet, promptReq)

	spans := rec.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	sentinels := []string{"secret-location", "secret.txt", "secret-code"}
	for _, s := range spans {
		testutil.AssertNoSentinel(t, s, sentinels)
		// Also verify no content attributes are present.
		testutil.AssertNoContentCaptured(t, s)
	}
}

// TestFullNestedScenario verifies a complete nested scenario:
// Agent → inference → MCP tools/call → inference
func TestFullNestedScenario(t *testing.T) {
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

	clientMW := ClientMiddleware(instr)
	clientHandler := clientMW(noopNext)

	// Start agent.
	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "test-agent",
		Model: "gemini-2.5-flash",
	})

	// First inference call.
	inferReq := otelgenai.Request{
		Operation: otelgenai.Operation("generate_content"),
		Model:     "gemini-2.5-flash",
		Provider: "gcp.gemini",
	}
	_, inferOp := instr.StartInference(ctx, inferReq)
	inferOp.End(otelgenai.Response{}, nil)

	// MCP tools/call.
	toolReq := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "get_weather"},
	}
	_, _ = clientHandler(ctx, MethodToolsCall, toolReq)

	// Second inference call.
	_, inferOp2 := instr.StartInference(ctx, inferReq)
	inferOp2.End(otelgenai.Response{}, nil)

	agentOp.End(nil)

	spans := rec.Spans()
	if len(spans) != 4 {
		t.Fatalf("expected 4 spans (agent + 2 inference + tool), got %d", len(spans))
	}

	// Verify agent hierarchy.
	testutil.AssertAgentHierarchy(t, spans, "test-agent", 2, 1)

	// Verify no content leakage.
	for _, s := range spans {
		testutil.AssertNoContentCaptured(t, s)
	}
}

// TestMCPErrorClassification verifies that MCP errors are classified
// consistently using the core classifier.
func TestMCPErrorClassification(t *testing.T) {
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

	// Test with context.Canceled.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	clientMW := ClientMiddleware(instr)
	cancelNext := func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, context.Canceled
	}
	handler := clientMW(cancelNext)

	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{Name: "test"},
	}
	_, _ = handler(cancelCtx, MethodToolsCall, req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// The error.type should be classified by the core DefaultClassifier.
	// context.Canceled → "cancelled"
	testutil.AssertErrorType(t, spans[0], string(otelgenai.ErrorTypeCancelled))
}

// TestGlobalPropagatorContract verifies that with a W3C propagator,
// client → server _meta propagation preserves the trace, while with
// a no-op propagator the adapter remains safe and does not mutate
// caller-owned metadata.
func TestGlobalPropagatorContract(t *testing.T) {
	// With W3C propagator: traceparent injected, server span is child of client.
	t.Run("W3C_propagator", func(t *testing.T) {
		defer setupTestPropagator()()

		rec := testutil.NewRecorder()
		defer rec.Shutdown(context.Background())

		instr, _ := otelgenai.New(
			otelgenai.WithTracerProvider(rec.TracerProvider()),
			otelgenai.WithMeterProvider(rec.MeterProvider()),
		)

		clientMW := ClientMiddleware(instr)
		clientHandler := clientMW(noopNext)

		req := &mcp.ClientRequest[*mcp.CallToolParams]{
			Params: &mcp.CallToolParams{Name: "test"},
		}
		_, _ = clientHandler(context.Background(), MethodToolsCall, req)

		meta := req.Params.GetMeta()
		if _, ok := meta["traceparent"]; !ok {
			t.Error("W3C propagator should inject traceparent")
		}
	})

	// With no-op propagator: no traceparent, but _meta is still cloned safely.
	t.Run("noop_propagator", func(t *testing.T) {
		origProp := otel.GetTextMapPropagator()
		defer otel.SetTextMapPropagator(origProp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

		rec := testutil.NewRecorder()
		defer rec.Shutdown(context.Background())

		instr, _ := otelgenai.New(
			otelgenai.WithTracerProvider(rec.TracerProvider()),
			otelgenai.WithMeterProvider(rec.MeterProvider()),
		)

		clientMW := ClientMiddleware(instr)
		clientHandler := clientMW(noopNext)

		originalMeta := mcp.Meta{"existing": "value"}
		req := &mcp.ClientRequest[*mcp.CallToolParams]{
			Params: &mcp.CallToolParams{Name: "test", Meta: originalMeta},
		}
		_, _ = clientHandler(context.Background(), MethodToolsCall, req)

		// No traceparent should be injected.
		meta := req.Params.GetMeta()
		if _, ok := meta["traceparent"]; ok {
			t.Error("no-op propagator should not inject traceparent")
		}

		// Existing value should be preserved.
		if v, ok := meta["existing"]; !ok || v != "value" {
			t.Error("existing _meta value not preserved with no-op propagator")
		}

		// Original map should not be mutated.
		if _, ok := originalMeta["traceparent"]; ok {
			t.Error("original map was mutated with no-op propagator")
		}
	})
}

// TestNoContentAttributes verifies that MCP-specific content attributes
// are not present on any span.
func TestNoContentAttributes(t *testing.T) {
	defer setupTestPropagator()()

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	instr, _ := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
	)

	clientMW := ClientMiddleware(instr)
	clientHandler := clientMW(noopNext)

	// tools/call with arguments
	req := &mcp.ClientRequest[*mcp.CallToolParams]{
		Params: &mcp.CallToolParams{
			Name:      "test",
			Arguments: map[string]any{"secret": "data"},
		},
	}
	_, _ = clientHandler(context.Background(), MethodToolsCall, req)

	spans := rec.Spans()
	for _, s := range spans {
		// Verify no tool call arguments or results attributes.
		testutil.AssertAttrNotPresent(t, s, "gen_ai.tool.call.arguments")
		testutil.AssertAttrNotPresent(t, s, "gen_ai.tool.call.result")
	}
}

// Ensure metricdata is used (for agent hierarchy metric checks).
var _ metricdata.Histogram[int64]

// Ensure attribute is used.
var _ attribute.KeyValue
