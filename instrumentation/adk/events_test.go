package adk

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// newEventPlugin creates a Plugin wired to a full testutil Recorder
// (traces, metrics, and logs) with a passthrough projector.
func newEventPlugin(t *testing.T, rec *testutil.Recorder, opts ...otelgenai.Option) *Plugin {
	t.Helper()
	instr, err := otelgenai.New(append([]otelgenai.Option{
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
		otelgenai.WithContentProjector(func(v otelgenai.ContentValue) (otelgenai.ContentValue, error) {
			return v, nil
		}),
	}, opts...)...)
	if err != nil {
		t.Fatalf("New instrumenter: %v", err)
	}
	return &Plugin{
		instr:    instr,
		registry: newRegistry(),
		system:   "adk",
		provider: "gcp.gen_ai",
	}
}

func eventAttrs(t *testing.T, rec sdklog.Record) map[string]attribute.Value {
	t.Helper()
	out := map[string]attribute.Value{}
	rec.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value
		return true
	})
	return out
}

func TestInferenceDetailsEventEmitted(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)

	ctx := newMockCtx("inv1", "root", "agent1").withSession("sess-42")
	p.beforeAgent(ctx)

	occurred := time.Now().Add(-time.Minute).UTC()
	p.beforeModel(ctx, &model.LLMRequest{
		Model: "gemini-2.5-flash",
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "weather in Paris?"}}},
		},
	}, occurred)

	p.afterModel(ctx, &model.LLMResponse{
		Content:      &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "rainy"}}},
		ModelVersion: "gemini-2.5-flash-001",
		FinishReason: genai.FinishReasonStop,
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}, nil, occurred)

	recs := rec.RecordsWithEventName("gen_ai.client.inference.operation.details")
	if len(recs) != 1 {
		t.Fatalf("inference details events = %d, want 1", len(recs))
	}
	r := recs[0]
	if !r.Timestamp().Equal(occurred) {
		t.Errorf("timestamp = %v, want occurrence time %v", r.Timestamp(), occurred)
	}
	attrs := eventAttrs(t, r)
	if got := attrs["gen_ai.provider.name"].AsString(); got != "gcp.gen_ai" {
		t.Errorf("provider.name = %q", got)
	}
	if got := attrs["gen_ai.conversation.id"].AsString(); got != "sess-42" {
		t.Errorf("conversation.id = %q, want session-derived %q", got, "sess-42")
	}
	if got := attrs["gen_ai.response.model"].AsString(); got != "gemini-2.5-flash-001" {
		t.Errorf("response.model = %q", got)
	}
	if got := attrs["gen_ai.usage.input_tokens"].AsInt64(); got != 10 {
		t.Errorf("input_tokens = %d", got)
	}
	out := attrs["gen_ai.output.messages"]
	if out.Type() != attribute.SLICE {
		t.Fatalf("output.messages type = %v", out.Type())
	}
}

func TestInferenceDetailsRequiresProvider(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)
	p.provider = "" // no provider configured

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{
		Model:    "gemini-2.5-flash",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}, time.Now())
	p.afterModel(ctx, &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}},
	}, nil, time.Now())

	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 without provider name", n)
	}
}

func TestInferenceDetailsProviderResolver(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)
	p.provider = ""
	p.providerResolver = func(modelName string) string {
		if modelName == "gemini-2.5-flash" {
			return "gcp.vertex_ai"
		}
		return ""
	}

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{
		Model:    "gemini-2.5-flash",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}, time.Now())
	p.afterModel(ctx, &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}},
	}, nil, time.Now())

	recs := rec.RecordsWithEventName("gen_ai.client.inference.operation.details")
	if len(recs) != 1 {
		t.Fatalf("events = %d, want 1", len(recs))
	}
	if got := eventAttrs(t, recs[0])["gen_ai.provider.name"].AsString(); got != "gcp.vertex_ai" {
		t.Errorf("provider.name = %q", got)
	}
}

func TestToolDetailsEventEmitted(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)

	ctx := newMockCtx("inv1", "root", "agent1").withSession("sess-7")
	p.beforeAgent(ctx)

	// Simulate an active execute_tool span.
	tracer := rec.TracerProvider().Tracer("test")
	_, span := tracer.Start(ctx, "execute_tool get_weather")
	mockWithSpan := ctx.withSpan(span)

	args := map[string]any{"location": "Paris"}
	p.beforeTool(mockWithSpan, &mockTool{name: "get_weather"}, args, time.Now())

	occurred := time.Now().UTC()
	p.afterTool(mockWithSpan, map[string]any{"temp": "57F"}, nil, occurred)
	span.End()

	recs := rec.RecordsWithEventName("otelgenai.execute_tool.operation.details")
	if len(recs) != 1 {
		t.Fatalf("tool details events = %d, want 1", len(recs))
	}
	r := recs[0]
	if !r.Timestamp().Equal(occurred) {
		t.Errorf("timestamp = %v, want %v", r.Timestamp(), occurred)
	}
	attrs := eventAttrs(t, r)
	if got := attrs["gen_ai.tool.name"].AsString(); got != "get_weather" {
		t.Errorf("tool.name = %q", got)
	}
	if got := attrs["gen_ai.conversation.id"].AsString(); got != "sess-7" {
		t.Errorf("conversation.id = %q", got)
	}
	argVal := attrs["gen_ai.tool.call.arguments"]
	if argVal.Type() != attribute.MAP {
		t.Fatalf("arguments type = %v, want MAP", argVal.Type())
	}
	// The event must be correlated to the tool span context.
	if r.SpanID() != span.SpanContext().SpanID() {
		t.Errorf("event span ID = %v, want tool span %v", r.SpanID(), span.SpanContext().SpanID())
	}
}

func TestModelErrorObservedAndContinuation(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	errAt := time.Now().Add(-2 * time.Second).UTC()
	p.onModelError(ctx, errors.New("provider timeout"), errAt)

	errRecs := rec.RecordsWithEventName("otelgenai.agent.model.error_observed")
	if len(errRecs) != 1 {
		t.Fatalf("error_observed events = %d, want 1", len(errRecs))
	}
	if !errRecs[0].Timestamp().Equal(errAt) {
		t.Errorf("error timestamp = %v, want %v", errRecs[0].Timestamp(), errAt)
	}
	if got := eventAttrs(t, errRecs[0])["gen_ai.agent.name"].AsString(); got != "agent1" {
		t.Errorf("agent.name = %q", got)
	}

	// The next model call emits continued_after_error at callback-entry time.
	contAt := time.Now().UTC()
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, contAt)

	contRecs := rec.RecordsWithEventName("otelgenai.agent.continued_after_error")
	if len(contRecs) != 1 {
		t.Fatalf("continued_after_error events = %d, want 1", len(contRecs))
	}
	if !contRecs[0].Timestamp().Equal(contAt) {
		t.Errorf("continuation timestamp = %v, want %v", contRecs[0].Timestamp(), contAt)
	}
	attrs := eventAttrs(t, contRecs[0])
	if got := attrs["gen_ai.operation.name"].AsString(); got != "generate_content" {
		t.Errorf("operation.name = %q", got)
	}
	if got := attrs["error.type"].AsString(); got != "unknown" {
		t.Errorf("error.type = %q, want classified prior error", got)
	}

	// A further model call must not emit a duplicate continuation.
	p.afterModel(ctx, &model.LLMResponse{}, nil, time.Now())
	p.beforeModel(ctx, &model.LLMRequest{Model: "gemini-2.5-flash"}, time.Now())
	if n := len(rec.RecordsWithEventName("otelgenai.agent.continued_after_error")); n != 1 {
		t.Fatalf("continued_after_error events = %d, want exactly 1", n)
	}
}

func TestToolErrorObserved(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)

	p.onToolError(ctx, errors.New("tool blew up"), time.Now())

	recs := rec.RecordsWithEventName("otelgenai.agent.tool.error_observed")
	if len(recs) != 1 {
		t.Fatalf("tool error events = %d, want 1", len(recs))
	}
	if got := eventAttrs(t, recs[0])["gen_ai.operation.name"].AsString(); got != "execute_tool" {
		t.Errorf("operation.name = %q", got)
	}
}

// TestToolErrorContinuation verifies that a pending tool error is consumed
// by the next beforeTool and emits exactly one continued_after_error with
// the execute_tool operation, the prior classified error, the session
// conversation ID, and the callback-entry timestamp.
func TestToolErrorContinuation(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)

	ctx := newMockCtx("inv1", "root", "agent1").withSession("sess-9")
	p.beforeAgent(ctx)

	errAt := time.Now().Add(-time.Second).UTC()
	p.onToolError(ctx, errors.New("tool blew up"), errAt)

	errRecs := rec.RecordsWithEventName("otelgenai.agent.tool.error_observed")
	if len(errRecs) != 1 {
		t.Fatalf("error_observed events = %d, want 1", len(errRecs))
	}

	// The next tool call emits continued_after_error at callback-entry time.
	tracer := rec.TracerProvider().Tracer("test")
	_, span := tracer.Start(ctx, "execute_tool next_tool")
	mockWithSpan := ctx.withSpan(span)

	contAt := time.Now().UTC()
	p.beforeTool(mockWithSpan, &mockTool{name: "next_tool"}, nil, contAt)

	contRecs := rec.RecordsWithEventName("otelgenai.agent.continued_after_error")
	if len(contRecs) != 1 {
		t.Fatalf("continued_after_error events = %d, want 1", len(contRecs))
	}
	if !contRecs[0].Timestamp().Equal(contAt) {
		t.Errorf("continuation timestamp = %v, want %v", contRecs[0].Timestamp(), contAt)
	}
	attrs := eventAttrs(t, contRecs[0])
	if got := attrs["gen_ai.operation.name"].AsString(); got != "execute_tool" {
		t.Errorf("operation.name = %q", got)
	}
	if got := attrs["error.type"].AsString(); got != "unknown" {
		t.Errorf("error.type = %q, want classified prior error", got)
	}
	if got := attrs["gen_ai.conversation.id"].AsString(); got != "sess-9" {
		t.Errorf("conversation.id = %q", got)
	}
	span.End()
}

func TestNoEventsWithoutContentProjector(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := &Plugin{
		instr:    instr,
		registry: newRegistry(),
		system:   "adk",
		provider: "gcp.gen_ai",
	}

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{
		Model:    "gemini-2.5-flash",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}, time.Now())
	p.afterModel(ctx, &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}},
	}, nil, time.Now())

	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 without projector", n)
	}
}

func TestOccurrenceEventsRequireNoProjector(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	instr, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := &Plugin{
		instr:    instr,
		registry: newRegistry(),
		system:   "adk",
	}

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.onModelError(ctx, errors.New("x"), time.Now())

	if n := len(rec.RecordsWithEventName("otelgenai.agent.model.error_observed")); n != 1 {
		t.Fatalf("error_observed = %d, want 1 without projector", n)
	}
}

func TestProviderResolverPanicIsolated(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	p := newEventPlugin(t, rec)
	p.provider = ""
	p.providerResolver = func(string) string { panic("boom") }

	ctx := newMockCtx("inv1", "root", "agent1")
	p.beforeAgent(ctx)
	p.beforeModel(ctx, &model.LLMRequest{
		Model:    "m",
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}, time.Now())
	p.afterModel(ctx, &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}},
	}, nil, time.Now())

	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 after resolver panic", n)
	}
}
