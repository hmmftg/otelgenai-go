package adk

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// mockCallbackContext implements callbackContext for testing.
type mockCallbackContext struct {
	context.Context
	invocationID string
	branch       string
	agentName    string
	sessionID    string
}

func (m *mockCallbackContext) InvocationID() string { return m.invocationID }
func (m *mockCallbackContext) Branch() string       { return m.branch }
func (m *mockCallbackContext) AgentName() string    { return m.agentName }
func (m *mockCallbackContext) SessionID() string    { return m.sessionID }

func newMockCtx(invocationID, branch, agentName string) *mockCallbackContext {
	return &mockCallbackContext{
		Context:      context.Background(),
		invocationID: invocationID,
		branch:       branch,
		agentName:    agentName,
	}
}

// withSession returns a copy carrying the given session ID.
func (m *mockCallbackContext) withSession(sessionID string) *mockCallbackContext {
	c := *m
	c.sessionID = sessionID
	return &c
}

// withSpan returns a new mockCallbackContext that carries the given
// span, preserving the callbackContext interface for agent key extraction.
func (m *mockCallbackContext) withSpan(span oteltrace.Span) *mockCallbackContext {
	return &mockCallbackContext{
		Context:      oteltrace.ContextWithSpan(m.Context, span),
		invocationID: m.invocationID,
		branch:       m.branch,
		agentName:    m.agentName,
		sessionID:    m.sessionID,
	}
}

// mockTool implements toolNameProvider for testing.
type mockTool struct {
	name string
}

func (m *mockTool) Name() string { return m.name }

// newTestPlugin creates a Plugin with test instrumentation and an optional
// diagnostic handler.
func newTestPlugin(t *testing.T, opts ...otelgenai.Option) *Plugin {
	t.Helper()
	instr := newTestInstrumenter(t, opts...)
	return &Plugin{
		instr:    instr,
		registry: newRegistry(),
		system:   "adk",
	}
}

// newTestPluginWithTracer creates a Plugin with a real tracer provider
// so active spans are available for tool tests.
func newTestPluginWithTracer(t *testing.T, exporter *tracetest.InMemoryExporter, instrOpts ...otelgenai.Option) (*Plugin, *trace.TracerProvider) {
	t.Helper()
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	instrOpts = append(instrOpts, otelgenai.WithTracerProvider(tp))
	instr := newTestInstrumenter(t, instrOpts...)
	return &Plugin{
		instr:    instr,
		registry: newRegistry(),
		system:   "adk",
	}, tp
}

// startToolSpan starts a span named "execute_tool" and returns a
// mockCallbackContext carrying the span plus the span itself.
// This preserves the callbackContext interface for agent key extraction.
func startToolSpan(tp *trace.TracerProvider, ctx *mockCallbackContext, name string) (*mockCallbackContext, oteltrace.Span) {
	tracer := tp.Tracer("test")
	_, span := tracer.Start(ctx, "execute_tool "+name, oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	return ctx.withSpan(span), span
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

func TestAfterRunReleasesAbandonedLifecycleState(t *testing.T) {
	p := newTestPlugin(t)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	// Simulate BeforeModel creating state but no AfterModel arriving.
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

	// AfterRun should clean up the abandoned model state.
	p.afterRun("inv1")

	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("abandoned model state not cleaned by AfterRun")
	}
	if p.registry.getAgent(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("agent state not cleaned by AfterRun")
	}
}

func TestStateCleanupWhenAnotherPluginIntercepts(t *testing.T) {
	p := newTestPlugin(t)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{Model: "model1"}, time.Now())

	// Simulate another plugin intercepting BeforeTool - no AfterTool arrives.
	// But we need a valid span context for tool state.
	// Since there's no active span, beforeTool fails closed (no state created).
	p.beforeTool(ctx, &mockTool{name: "tool1"}, nil, time.Now())

	// AfterRun should clean up abandoned agent and model state.
	p.afterRun("inv1")

	if p.registry.getAgent(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("agent state not cleaned")
	}
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("model state not cleaned")
	}
}

func TestBeforeModelCreatesState(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

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
		otelgenai.WithDiagnosticHandler(func(d otelgenai.Diagnostic) {
			if d.Reason == otelgenai.DiagnosticReasonModelStateConflict {
				diags.Add(1)
			}
		}),
	)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	p.beforeModel(ctx, &model.LLMRequest{Model: "model1"}, time.Now())
	// Second beforeModel should collide.
	p.beforeModel(ctx, &model.LLMRequest{Model: "model2"}, time.Now())

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
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

	// Terminal response (non-partial).
	usage := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     100,
		CandidatesTokenCount: 200,
		ThoughtsTokenCount:   50,
	}
	p.afterModel(ctx, &model.LLMResponse{UsageMetadata: usage, Partial: false}, nil, time.Now())

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
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

	// Partial response.
	usage := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     50,
		CandidatesTokenCount: 100,
	}
	p.afterModel(ctx, &model.LLMResponse{UsageMetadata: usage, Partial: true}, nil, time.Now())

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
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

	// Terminal with error.
	p.afterModel(ctx, &model.LLMResponse{UsageMetadata: nil, Partial: false}, errors.New("model error"), time.Now())

	// State should be deleted.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) != nil {
		t.Fatal("model state should be deleted after error terminal")
	}
}

func TestOnModelErrorIsObservational(t *testing.T) {
	p := newTestPlugin(t)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())

	// onModelError should not terminalize.
	p.onModelError(ctx, nil, time.Now())

	// State should still exist.
	if p.registry.getModel(agentStateFrom("inv1", "root", "agent1")) == nil {
		t.Fatal("model state should remain after onModelError")
	}
}

func TestBeforeToolInvalidSpanFailsClosed(t *testing.T) {
	var diags atomic.Int32
	p := newTestPlugin(t,
		otelgenai.WithDiagnosticHandler(func(d otelgenai.Diagnostic) {
			if d.Reason == otelgenai.DiagnosticReasonInvalidToolSpan {
				diags.Add(1)
			}
		}),
	)
	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	// Context without an active span - should fail closed.
	p.beforeTool(ctx, &mockTool{name: "tool1"}, nil, time.Now())

	if diags.Load() != 1 {
		t.Fatalf("expected 1 invalid tool span diagnostic, got %d", diags.Load())
	}

	// No tool state should be created.
	if len(p.registry.tools) != 0 {
		t.Fatalf("expected 0 tool states, got %d", len(p.registry.tools))
	}

	// Agent tool count should NOT be incremented.
	st := p.registry.getAgent(agentStateFrom("inv1", "root", "agent1"))
	if st.toolCalls != 0 {
		t.Fatalf("toolCalls = %d, want 0", st.toolCalls)
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

	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())

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
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())

	// AfterTool with error.
	p.afterTool(spanCtx, nil, errors.New("tool error"), time.Now())
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
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())

	// AfterTool with no error.
	p.afterTool(spanCtx, nil, nil, time.Now())
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
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())

	// onToolError should not terminalize.
	p.onToolError(spanCtx, nil, time.Now())

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
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())
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
	p, tp := newTestPluginWithTracer(t, exporter)
	p.toolSystemResolver = func(t toolNameProvider) string {
		return "resolved-system"
	}

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())
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
		otelgenai.WithDiagnosticHandler(func(d otelgenai.Diagnostic) {
			if d.Reason == otelgenai.DiagnosticReasonToolSystemResolverPanic {
				diags.Add(1)
			}
		}),
	)
	p.toolSystemResolver = func(t toolNameProvider) string {
		panic("resolver boom")
	}

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	spanCtx, span := startToolSpan(tp, ctx, "tool1")
	// Should not panic.
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())
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
	p.beforeTool(spanCtx, &mockTool{name: "tool1"}, nil, time.Now())
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

	// BeforeModel returns nothing (void).
	p.beforeModel(ctx, &model.LLMRequest{Model: "model"}, time.Now())

	// AfterModel returns nothing (void).
	p.afterModel(ctx, &model.LLMResponse{UsageMetadata: nil, Partial: false}, nil, time.Now())

	// OnModelError returns nothing (void).
	p.onModelError(ctx, nil, time.Now())

	// BeforeTool returns nothing (void).
	p.beforeTool(ctx, &mockTool{name: "tool"}, nil, time.Now())

	// AfterTool returns nothing (void).
	p.afterTool(ctx, nil, nil, time.Now())

	// OnToolError returns nothing (void).
	p.onToolError(ctx, nil, time.Now())
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
			p.beforeTool(spanCtx, &mockTool{name: "tool"}, nil, time.Now())
			p.afterTool(spanCtx, nil, nil, time.Now())
			span.End()

			p.afterAgent(ctx)
		}(i)
	}
	wg.Wait()
}

func TestMapUsageMetadata(t *testing.T) {
	um := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        100,
		CandidatesTokenCount:    200,
		ThoughtsTokenCount:      50,
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

func TestRegistryEndAgentReturnsDuration(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}

	r.startAgent(key, "agent1")
	time.Sleep(1 * time.Millisecond)
	st := r.endAgent(key)
	if st == nil {
		t.Fatal("endAgent returned nil")
	}
	if time.Since(st.start) <= 0 {
		t.Fatal("duration should be positive")
	}
}
