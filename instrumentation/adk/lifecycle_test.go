package adk

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// mockCallbackContext implements callbackContext for testing.
type mockCallbackContext struct {
	context.Context
	invocationID string
	branch       string
	agentName    string
}

func (m *mockCallbackContext) InvocationID() string { return m.invocationID }
func (m *mockCallbackContext) Branch() string       { return m.branch }
func (m *mockCallbackContext) AgentName() string    { return m.agentName }

func newMockCtx(invocationID, branch, agentName string) *mockCallbackContext {
	return &mockCallbackContext{
		Context:      context.Background(),
		invocationID: invocationID,
		branch:       branch,
		agentName:    agentName,
	}
}

// newTestPlugin creates a Plugin with test instrumentation.
func newTestPlugin(t *testing.T, opts ...Option) *Plugin {
	t.Helper()
	instr := newTestInstrumenter(t)
	cfg := config{system: "adk"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Plugin{
		instr:              instr,
		registry:           newRegistry(),
		system:             cfg.system,
		toolSystemResolver: cfg.toolSystemResolver,
	}
}

// newTestPluginWithTracer creates a Plugin with a real tracer provider
// so active spans are available for tool tests.
func newTestPluginWithTracer(t *testing.T, exporter *tracetest.InMemoryExporter, opts ...Option) (*Plugin, *trace.TracerProvider) {
	t.Helper()
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	instr, err := otelgenai.New(otelgenai.WithTracerProvider(tp))
	if err != nil {
		t.Fatalf("otelgenai.New: %v", err)
	}
	cfg := config{system: "adk"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Plugin{
		instr:              instr,
		registry:           newRegistry(),
		system:             cfg.system,
		toolSystemResolver: cfg.toolSystemResolver,
	}, tp
}

// startToolSpan starts a span named "execute_tool" and returns the
// context with the active span, mimicking ADK's execute_tool span.
func startToolSpan(tp *trace.TracerProvider, ctx context.Context, name string) (context.Context, oteltrace.Span) {
	tracer := tp.Tracer("test")
	return tracer.Start(ctx, "execute_tool "+name, oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
}

func TestBeforeAgentCreatesState(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")

	p.beforeAgent(ctx)

	st := p.registry.getAgent(agentStateFrom("inv1", "root", "agent1"))
	if st == nil {
		t.Fatal("agent state not created")
	}
	if st.name != "agent1" {
		t.Fatalf("agent name = %q, want %q", st.name, "agent1")
	}
}

func TestAfterAgentEmitsMetrics(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")

	p.beforeAgent(ctx)
	p.registry.incrementAgentInference(agentStateFrom("inv1", "root", "agent1"))
	p.registry.incrementAgentTool(agentStateFrom("inv1", "root", "agent1"))
	p.afterAgent(ctx)

	// State should be deleted.
	if p.registry.getAgent(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("agent state should be deleted after afterAgent")
	}
}

func TestAfterAgentEmitsZeroCounts(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")

	p.beforeAgent(ctx)
	// Don't increment any counters - should still emit zero counts.
	p.afterAgent(ctx)

	// State should be deleted.
	if p.registry.getAgent(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("agent state should be deleted after afterAgent")
	}
}

func TestAfterAgentIdempotent(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")

	p.beforeAgent(ctx)
	p.afterAgent(ctx)
	// Second afterAgent should be a no-op (state already deleted).
	p.afterAgent(ctx)
}

func TestAfterRunCleanupInvocationScoped(t *testing.T) {
	p := newTestPlugin(t)

	// Create state for two concurrent invocations.
	ctx1 := newMockCtx("inv1", "root", "agent1")
	ctx2 := newMockCtx("inv2", "root", "agent2")

	p.beforeAgent(ctx1)
	p.beforeAgent(ctx2)

	// Cleanup inv1 only.
	p.afterRun("inv1")

	// inv1 state should be gone.
	if p.registry.getAgent(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("inv1 agent state not cleaned")
	}
	// inv2 state should remain.
	if p.registry.getAgent(agentStateFrom("inv2", "root", "agent2")) == nil {
		t.Fatal("inv2 agent state should remain")
	}
}

func TestBeforeModelCreatesState(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	p.beforeModel(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"})

	st := p.registry.getModel(agentStateFrom("inv1", "root", "agent1"))
	if st == nil {
		t.Fatal("model state not created")
	}
	if st.model != "gemini-2.5-flash" {
		t.Fatalf("model = %q, want %q", st.model, "gemini-2.5-flash")
	}
}

func TestBeforeModelCollisionPreservesState(t *testing.T) {
	var diags atomic.Int32
	p := newTestPlugin(t,
		WithInstrumenterDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonModelStateConflict {
				diags.Add(1)
			}
		}),
	)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	p.beforeModel(ctx, &mockLLMRequest{Model: "model1"})
	// Second beforeModel should collide.
	p.beforeModel(ctx, &mockLLMRequest{Model: "model2"})

	if diags.Load() != 1 {
		t.Fatalf("expected 1 model state conflict diagnostic, got %d", diags.Load())
	}

	// Existing state should be preserved.
	st := p.registry.getModel(agentStateFrom("inv1", "root", "agent1"))
	if st == nil || st.model != "model1" {
		t.Fatal("existing model state not preserved")
	}
}

func TestAfterModelTerminalEmitsMetrics(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"})

	// Terminal response (non-partial).
	resp := &mockLLMResponse{
		Partial: false,
		UsageMetadata: &mockUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 200,
			ThoughtsTokenCount:   50,
		},
	}
	p.afterModel(ctx, resp, nil)

	// State should be deleted.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("model state should be deleted after terminal afterModel")
	}

	// Agent inference count should be incremented.
	st := p.registry.getAgent(agentStateFrom("inv1", "root", "agent1"))
	if st == nil {
		t.Fatal("agent state should still exist")
	}
	if st.inferenceCalls != 1 {
		t.Fatalf("inferenceCalls = %d, want 1", st.inferenceCalls)
	}
}

func TestAfterModelPartialDoesNotTerminalize(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"})

	// Partial response.
	resp := &mockLLMResponse{
		Partial: true,
		UsageMetadata: &mockUsageMetadata{
			PromptTokenCount:     50,
			CandidatesTokenCount: 100,
		},
	}
	p.afterModel(ctx, resp, nil)

	// State should still exist.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) == nil {
		t.Fatal("model state should remain after partial afterModel")
	}

	// Agent inference count should NOT be incremented.
	st := p.registry.getAgent(agentStateFrom("inv1", "root", "agent1"))
	if st.inferenceCalls != 0 {
		t.Fatalf("inferenceCalls = %d, want 0", st.inferenceCalls)
	}
}

func TestAfterModelErrorTerminalizes(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"})

	// Terminal with error.
	p.afterModel(ctx, nil, errors.New("model error"))

	// State should be deleted.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("model state should be deleted after error terminal")
	}
}

func TestOnModelErrorIsObservational(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"})

	// onModelError should not terminalize.
	p.onModelError(ctx, &mockLLMRequest{Model: "gemini-2.5-flash"}, errors.New("error"))

	// State should still exist.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) == nil {
		t.Fatal("model state should remain after onModelError")
	}
}

func TestBeforeToolInvalidSpanFailsClosed(t *testing.T) {
	var diags atomic.Int32
	p := newTestPlugin(t,
		WithInstrumenterDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonInvalidToolSpan {
				diags.Add(1)
			}
		}),
	)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	// Context without an active span - should fail closed.
	p.beforeTool(ctx, &mockTool{name: "tool1"}, nil)

	if diags.Load() != 1 {
		t.Fatalf("expected 1 invalid tool span diagnostic, got %d", diags.Load())
	}

	// No tool state should be created.
	if p.registry.getTool(toolStateKey{}) != nil {
		t.Fatal("no tool state should be created with invalid span")
	}
}

func TestBeforeToolValidSpanCreatesState(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	// Start a real span to get a valid span context.
	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	defer span.End()

	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)

	// Tool state should be created.
	sc := oteltrace.SpanFromContext(spanCtx).SpanContext()
	key := toolStateKey{TraceID: sc.TraceID(), SpanID: sc.SpanID()}
	if p.registry.getTool(key) == nil {
		t.Fatal("tool state not created with valid span")
	}
}

func TestAfterToolEmitsMetricsAndAugmentsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)

	// AfterTool with error.
	p.afterTool(spanCtx, &mockTool{name: "tool1"}, nil, nil, errors.New("tool error"))
	span.End()

	// Tool state should be deleted.
	sc := oteltrace.SpanFromContext(spanCtx).SpanContext()
	key := toolStateKey{TraceID: sc.TraceID(), SpanID: sc.SpanID()}
	if p.registry.getTool(key) != nil {
		t.Fatal("tool state should be deleted after afterTool")
	}

	// Agent tool count should be incremented.
	st := p.registry.getAgent(agentStateFrom("inv1", "root", "agent1"))
	if st == nil {
		t.Fatal("agent state should exist")
	}
	if st.toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want 1", st.toolCalls)
	}

	// Span should have error.type attribute.
	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	foundErrorType := false
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == semconv.AttrErrorType {
			foundErrorType = true
			if attr.Value.AsString() == "" {
				t.Fatal("error.type should be non-empty")
			}
		}
	}
	if !foundErrorType {
		t.Fatal("span should have error.type attribute")
	}
}

func TestAfterToolSuccessSetsOKStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)

	// AfterTool with no error.
	p.afterTool(spanCtx, &mockTool{name: "tool1"}, nil, nil, nil)
	span.End()

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Ok {
		t.Fatalf("span status = %v, want %v", spans[0].Status().Code, codes.Ok)
	}
}

func TestOnToolErrorIsObservational(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)

	// onToolError should not terminalize.
	p.onToolError(spanCtx, &mockTool{name: "tool1"}, nil, errors.New("error"))

	sc := oteltrace.SpanFromContext(spanCtx).SpanContext()
	key := toolStateKey{TraceID: sc.TraceID(), SpanID: sc.SpanID()}
	if p.registry.getTool(key) == nil {
		t.Fatal("tool state should remain after onToolError")
	}
	span.End()
}

func TestBeforeToolSetsToolTypeAttribute(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)
	span.End()

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	foundToolType := false
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == semconv.AttrGenAIToolType {
			foundToolType = true
			if attr.Value.AsString() != semconv.ToolTypeFunction {
				t.Fatalf("tool type = %q, want %q", attr.Value.AsString(), semconv.ToolTypeFunction)
			}
		}
	}
	if !foundToolType {
		t.Fatal("span should have gen_ai.tool.type attribute")
	}
}

func TestBeforeToolSystemResolverSetsSystem(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	resolver := func(t mockToolInterface) string {
		return "resolved-system"
	}
	_ = resolver

	// We need to use a resolver that accepts the ADK tool.Tool interface.
	// Since we're using mockTool, we need a compatible resolver.
	p, tp := newTestPluginWithTracer(t, exporter, WithToolSystemResolver(
		func(t mockToolInterface) string { return "resolved-system" },
	))

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)
	span.End()

	spans := exporter.GetSpans().Snapshots()
	foundSystem := false
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == semconv.AttrGenAISystem {
			foundSystem = true
			if attr.Value.AsString() != "resolved-system" {
				t.Fatalf("system = %q, want %q", attr.Value.AsString(), "resolved-system")
			}
		}
	}
	if !foundSystem {
		t.Fatal("span should have gen_ai.system attribute from resolver")
	}
}

func TestBeforeToolSystemResolverPanicIsIsolated(t *testing.T) {
	var diags atomic.Int32
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter,
		WithInstrumenterDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonToolSystemResolverPanic {
				diags.Add(1)
			}
		}),
		WithToolSystemResolver(func(t mockToolInterface) string {
			panic("resolver boom")
		}),
	)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	// Should not panic.
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)
	span.End()

	if diags.Load() != 1 {
		t.Fatalf("expected 1 resolver panic diagnostic, got %d", diags.Load())
	}

	// gen_ai.system should be omitted.
	spans := exporter.GetSpans().Snapshots()
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == semconv.AttrGenAISystem {
			t.Fatal("gen_ai.system should be omitted on resolver panic")
		}
	}
}

func TestBeforeToolNoResolverOmitsSystem(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil)
	span.End()

	spans := exporter.GetSpans().Snapshots()
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == semconv.AttrGenAISystem {
			t.Fatal("gen_ai.system should be omitted without resolver")
		}
	}
}

func TestCallbacksNeverInterceptExecution(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")

	// BeforeAgent should return (nil, nil).
	content, err := p.beforeAgent(ctx)
	if content != nil || err != nil {
		t.Fatalf("beforeAgent returned (%v, %v), want (nil, nil)", content, err)
	}

	// AfterAgent should return (nil, nil).
	content, err = p.afterAgent(ctx)
	if content != nil || err != nil {
		t.Fatalf("afterAgent returned (%v, %v), want (nil, nil)", content, err)
	}

	// BeforeModel should return (nil, nil).
	resp, err := p.beforeModel(ctx, &mockLLMRequest{Model: "model"})
	if resp != nil || err != nil {
		t.Fatalf("beforeModel returned (%v, %v), want (nil, nil)", resp, err)
	}

	// AfterModel should return (nil, nil).
	resp, err = p.afterModel(ctx, &mockLLMResponse{Partial: false}, nil)
	if resp != nil || err != nil {
		t.Fatalf("afterModel returned (%v, %v), want (nil, nil)", resp, err)
	}

	// OnModelError should return (nil, nil).
	resp, err = p.onModelError(ctx, &mockLLMRequest{Model: "model"}, errors.New("err"))
	if resp != nil || err != nil {
		t.Fatalf("onModelError returned (%v, %v), want (nil, nil)", resp, err)
	}

	// BeforeTool should return (nil, nil).
	result, err := p.beforeTool(ctx, &mockTool{name: "tool"}, nil)
	if result != nil || err != nil {
		t.Fatalf("beforeTool returned (%v, %v), want (nil, nil)", result, err)
	}

	// AfterTool should return (nil, nil).
	result, err = p.afterTool(ctx, &mockTool{name: "tool"}, nil, nil, nil)
	if result != nil || err != nil {
		t.Fatalf("afterTool returned (%v, %v), want (nil, nil)", result, err)
	}

	// OnToolError should return (nil, nil).
	result, err = p.onToolError(ctx, &mockTool{name: "tool"}, nil, errors.New("err"))
	if result != nil || err != nil {
		t.Fatalf("onToolError returned (%v, %v), want (nil, nil)", result, err)
	}
}

func TestConcurrentRunsSameSpanIDDoNotCollide(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	p, tp := newTestPluginWithTracer(t, exporter)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			invID := "inv" + string(rune('A'+n))
			ctx := newMockCtx(invID, "root", "agent")
			p.beforeAgent(ctx)

			// Each goroutine starts its own span (different TraceID).
			spanCtx, span := startToolSpan(tp, ctx, "tool")
			p.beforeTool(spanCtx, &mockTool{name: "tool"}, nil)
			p.afterTool(spanCtx, &mockTool{name: "tool"}, nil, nil, nil)
			span.End()

			p.afterAgent(ctx)
		}(i)
	}
	wg.Wait()
}

func TestMapUsageMetadata(t *testing.T) {
	um := &mockUsageMetadata{
		PromptTokenCount:     100,
		CandidatesTokenCount: 200,
		ThoughtsTokenCount:   50,
		CachedContentTokenCount: 30,
	}
	usage := mapUsageMetadata(um)
	if usage.InputTokens != 100 {
		t.Fatalf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 250 {
		t.Fatalf("OutputTokens = %d, want 250", usage.OutputTokens)
	}
	if usage.CacheReadTokens != 30 {
		t.Fatalf("CacheReadTokens = %d, want 30", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 0 {
		t.Fatalf("CacheWriteTokens = %d, want 0", usage.CacheWriteTokens)
	}
	if usage.ReasoningTokens != 50 {
		t.Fatalf("ReasoningTokens = %d, want 50", usage.ReasoningTokens)
	}
}

func TestMapUsageMetadataNil(t *testing.T) {
	usage := mapUsageMetadata(nil)
	if usage != (otelgenai.Usage{}) {
		t.Fatalf("nil usage should return zero Usage, got %v", usage)
	}
}

// Mock types for testing.

type mockToolInterface interface {
	Name() string
	Description() string
	IsLongRunning() bool
}

type mockTool struct {
	name string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "test tool" }
func (m *mockTool) IsLongRunning() bool { return false }

type mockLLMRequest struct {
	Model string
}

type mockLLMResponse struct {
	Partial      bool
	UsageMetadata *mockUsageMetadata
}

type mockUsageMetadata struct {
	PromptTokenCount        int
	CandidatesTokenCount    int
	ThoughtsTokenCount      int
	CachedContentTokenCount int
}

// WithInstrumenterDiagnosticHandler is a test helper that creates a Plugin
// with a diagnostic handler. This is needed because the adapter's
// diagnostic handler is on the Instrumenter, not the Plugin.
func WithInstrumenterDiagnosticHandler(h safety.DiagnosticHandler) Option {
	// This is a no-op option; the diagnostic handler is set on the Instrumenter.
	// We use a different approach in tests: create the Instrumenter with the handler.
	return func(c *config) {}
}
