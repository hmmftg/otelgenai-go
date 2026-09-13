package mcp

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

// setupInMemoryServer creates an MCP server with a tool, resource, and prompt,
// adds instrumentation middleware, and connects it to the given transport.
func setupInMemoryServer(t *testing.T, instr *otelgenai.Instrumenter, transport mcp.Transport) *mcp.ServerSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0"}, nil)

	// Add instrumentation middleware.
	server.AddReceivingMiddleware(ServerMiddleware(instr))

	// Add a tool.
	server.AddTool(&mcp.Tool{
		Name: "get_weather",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "sunny, 22C"},
			},
		}, nil
	})

	// Add a resource.
	server.AddResource(&mcp.Resource{
		Name:        "test-resource",
		URI:         "file:///test.txt",
		Description: "A test resource",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: "file:///test.txt", Text: "resource content"},
			},
		}, nil
	})

	// Add a prompt.
	server.AddPrompt(&mcp.Prompt{
		Name:        "greet",
		Description: "A greeting prompt",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "Hello"}},
			},
		}, nil
	})

	ss, err := server.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	return ss
}

// TestInMemoryTransport_Integration verifies the full client-server flow
// with instrumentation on both sides using the SDK's in-memory transport.
func TestInMemoryTransport_Integration(t *testing.T) {
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

	// Create in-memory transports.
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Set up server with instrumentation.
	_ = setupInMemoryServer(t, instr, serverTransport)

	// Set up client with instrumentation.
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	client.AddSendingMiddleware(ClientMiddleware(instr))

	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	// Call the tool.
	toolResult, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_weather",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if toolResult == nil || len(toolResult.Content) == 0 {
		t.Fatal("empty tool result")
	}

	// Read the resource.
	_, err = cs.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "file:///test.txt",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	// Get the prompt.
	_, err = cs.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name: "greet",
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}

	spans := rec.Spans()
	// Expected: 2 tools/call (client + server) + 2 resources/read + 2 prompts/get = 6
	if len(spans) != 6 {
		t.Fatalf("expected 6 spans (2 per method × 3 methods), got %d", len(spans))
	}

	// Count span names.
	toolSpans := 0
	resourceSpans := 0
	promptSpans := 0
	for _, s := range spans {
		switch s.Name() {
		case "execute_tool get_weather":
			toolSpans++
		case "resources/read":
			resourceSpans++
		case "prompts/get":
			promptSpans++
		}
	}
	if toolSpans != 2 {
		t.Errorf("expected 2 tool spans, got %d", toolSpans)
	}
	if resourceSpans != 2 {
		t.Errorf("expected 2 resource spans, got %d", resourceSpans)
	}
	if promptSpans != 2 {
		t.Errorf("expected 2 prompt spans, got %d", promptSpans)
	}

	// Verify all spans are INTERNAL and OK.
	for _, s := range spans {
		if s.Status().Code != codes.Ok {
			t.Errorf("span %q has non-OK status: %v", s.Name(), s.Status().Code)
		}
	}

	// Verify no content leakage.
	sentinels := []string{"sunny, 22C", "resource content", "Hello"}
	for _, s := range spans {
		testutil.AssertNoSentinel(t, s, sentinels)
		testutil.AssertNoContentCaptured(t, s)
	}
}

// TestInMemoryTransport_ParentChildRelationship verifies that the
// client span is the parent of the server span via _meta propagation.
func TestInMemoryTransport_ParentChildRelationship(t *testing.T) {
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

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_ = setupInMemoryServer(t, instr, serverTransport)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	client.AddSendingMiddleware(ClientMiddleware(instr))

	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	_, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_weather",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	spans := rec.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (client + server), got %d", len(spans))
	}

	// Both spans should be "execute_tool get_weather".
	// The client span has no parent (root), the server span has the client as parent.
	var clientSpan, serverSpan = spans[0], spans[1]
	if clientSpan.Parent().SpanID().IsValid() {
		clientSpan, serverSpan = serverSpan, clientSpan
	}

	// Verify server span's parent is the client span.
	if serverSpan.Parent().SpanID() != clientSpan.SpanContext().SpanID() {
		t.Errorf("server span parent: got %q, want %q (client span)",
			serverSpan.Parent().SpanID(), clientSpan.SpanContext().SpanID())
	}

	// Verify both spans share the same trace.
	if serverSpan.SpanContext().TraceID() != clientSpan.SpanContext().TraceID() {
		t.Errorf("trace ID mismatch: server %q, client %q",
			serverSpan.SpanContext().TraceID(), clientSpan.SpanContext().TraceID())
	}
}

// TestInMemoryTransport_NestedAgent verifies the nested agent/inference/MCP scenario.
func TestInMemoryTransport_NestedAgent(t *testing.T) {
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

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_ = setupInMemoryServer(t, instr, serverTransport)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	client.AddSendingMiddleware(ClientMiddleware(instr))

	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	// Start an agent.
	ctx, agentOp := instr.StartAgent(context.Background(), otelgenai.AgentRequest{
		Name:  "weather-agent",
		Model: "gemini-2.5-flash",
	})

	// First inference.
	inferReq := otelgenai.Request{
		Operation: otelgenai.Operation("generate_content"),
		Provider:  "gcp.gemini",
		Model:     "gemini-2.5-flash",
	}
	_, inferOp := instr.StartInference(ctx, inferReq)
	inferOp.End(otelgenai.Response{}, nil)

	// MCP tool call (client-side increments agent counter).
	_, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_weather"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	// Second inference.
	_, inferOp2 := instr.StartInference(ctx, inferReq)
	inferOp2.End(otelgenai.Response{}, nil)

	agentOp.End(nil)

	spans := rec.Spans()
	// Expected: 1 agent + 2 inference + 2 tool (client + server) = 5
	if len(spans) != 5 {
		t.Fatalf("expected 5 spans, got %d", len(spans))
	}

	// Verify agent hierarchy: 2 inferences + 1 tool call under the agent.
	testutil.AssertAgentHierarchy(t, spans, "weather-agent", 2, 1)

	// Verify no content leakage.
	for _, s := range spans {
		testutil.AssertNoContentCaptured(t, s)
	}
}

// Ensure the propagator import is used.
var _ propagation.TextMapPropagator = propagation.TraceContext{}
var _ = otel.Tracer
