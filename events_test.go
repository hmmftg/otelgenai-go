package otelgenai_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	nooplog "go.opentelemetry.io/otel/log/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func passthroughProjector(v otelgenai.ContentValue) (otelgenai.ContentValue, error) {
	return v, nil
}

func newEventInstrumenter(t *testing.T, rec *testutil.Recorder, opts ...otelgenai.Option) *otelgenai.Instrumenter {
	t.Helper()
	all := []otelgenai.Option{
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
		otelgenai.WithContentProjector(passthroughProjector),
	}
	all = append(all, opts...)
	in, err := otelgenai.New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return in
}

func recordAttrs(rec sdklog.Record) map[string]attribute.Value {
	out := map[string]attribute.Value{}
	rec.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value
		return true
	})
	return out
}

// mapLookup finds a key in a MAP-type attribute.Value.
func mapLookup(v attribute.Value, key string) (attribute.Value, bool) {
	if v.Type() != attribute.MAP {
		return attribute.Value{}, false
	}
	for _, kv := range v.AsMap() {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestEmitInferenceDetails(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	ctx := otelgenai.WithConversationID(context.Background(), "conv-1")
	occurred := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)

	in.EmitInferenceDetails(ctx, otelgenai.InferenceDetails{
		Operation:    otelgenai.Operation("generate_content"),
		Provider:     "gcp.gen_ai",
		RequestModel: "gemini-2.5-flash",
		Usage:        otelgenai.Usage{InputTokens: 10, OutputTokens: 5},
		InputMessages: in.ProjectContent(otelgenai.ContentValue{
			Kind: otelgenai.ContentKindInputMessages,
			Messages: []otelgenai.Message{{
				Role:  "user",
				Parts: []otelgenai.MessagePart{{Type: otelgenai.MessagePartText, Content: "hi"}},
			}},
		}),
		OccurredAt: occurred,
	})

	recs := rec.RecordsWithEventName("gen_ai.client.inference.operation.details")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	r := recs[0]
	if !r.Timestamp().Equal(occurred) {
		t.Errorf("timestamp = %v, want %v (occurrence time)", r.Timestamp(), occurred)
	}
	attrs := recordAttrs(r)
	if got := attrs["gen_ai.operation.name"].AsString(); got != "generate_content" {
		t.Errorf("operation.name = %q", got)
	}
	if got := attrs["gen_ai.provider.name"].AsString(); got != "gcp.gen_ai" {
		t.Errorf("provider.name = %q", got)
	}
	if got := attrs["gen_ai.conversation.id"].AsString(); got != "conv-1" {
		t.Errorf("conversation.id = %q", got)
	}
	if got := attrs["gen_ai.usage.input_tokens"].AsInt64(); got != 10 {
		t.Errorf("input_tokens = %d", got)
	}
	msgs := attrs["gen_ai.input.messages"]
	if msgs.Type() != attribute.SLICE {
		t.Fatalf("input.messages type = %v, want SLICE", msgs.Type())
	}
	role, ok := mapLookup(msgs.AsSlice()[0], "role")
	if !ok || role.AsString() != "user" {
		t.Error("input.messages[0].role != user")
	}
}

func TestEmitInferenceDetailsNoContentNoEvent(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	in.EmitInferenceDetails(context.Background(), otelgenai.InferenceDetails{
		Operation: "generate_content",
		Provider:  "openai",
	})
	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 (metadata-only details are not emitted)", n)
	}
}

func TestEmitInferenceDetailsRequiresProvider(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	content := in.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: []otelgenai.Message{{Role: "user", Content: "hi"}},
	})
	in.EmitInferenceDetails(context.Background(), otelgenai.InferenceDetails{
		Operation:     "generate_content",
		InputMessages: content,
	})
	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 without provider", n)
	}
}

func TestEmitToolDetails(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	occurred := time.Now().Add(-30 * time.Second).UTC()
	in.EmitToolDetails(context.Background(), otelgenai.ToolDetails{
		ToolName: "get_weather",
		Arguments: in.ProjectContent(otelgenai.ContentValue{
			Kind:              otelgenai.ContentKindToolCallArguments,
			ToolCallArguments: `{"location":"Paris"}`,
		}),
		Result: in.ProjectContent(otelgenai.ContentValue{
			Kind:           otelgenai.ContentKindToolCallResult,
			ToolCallResult: `{"temp":"57F"}`,
		}),
		OccurredAt: occurred,
	})

	recs := rec.RecordsWithEventName("otelgenai.execute_tool.operation.details")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	r := recs[0]
	if !r.Timestamp().Equal(occurred) {
		t.Errorf("timestamp = %v, want occurrence time %v", r.Timestamp(), occurred)
	}
	attrs := recordAttrs(r)
	args := attrs["gen_ai.tool.call.arguments"]
	if args.Type() != attribute.MAP {
		t.Fatalf("arguments type = %v, want MAP", args.Type())
	}
	if loc, ok := mapLookup(args, "location"); !ok || loc.AsString() != "Paris" {
		t.Errorf("arguments.location missing or wrong")
	}
}

func TestEmitToolDetailsErrorOnly(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	in.EmitToolDetails(context.Background(), otelgenai.ToolDetails{
		ToolName:  "flaky",
		ErrorType: otelgenai.ErrorTypeUnknown,
	})
	recs := rec.RecordsWithEventName("otelgenai.execute_tool.operation.details")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1 for error observation", len(recs))
	}
	if got := recordAttrs(recs[0])["error.type"].AsString(); got != "unknown" {
		t.Errorf("error.type = %q", got)
	}
	if recs[0].Severity() != otellog.SeverityError {
		t.Errorf("severity = %v, want ERROR", recs[0].Severity())
	}
}

func TestEmitAgentOccurrence(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	occurred := time.Now().Add(-time.Second).UTC()
	in.EmitAgentOccurrence(context.Background(), otelgenai.AgentOccurrence{
		Kind:           otelgenai.OccurrenceModelErrorObserved,
		AgentName:      "assistant",
		Operation:      otelgenai.Operation("generate_content"),
		ConversationID: "conv-9",
		ErrorType:      otelgenai.ErrorTypeUnknown,
		OccurredAt:     occurred,
	})

	recs := rec.RecordsWithEventName("otelgenai.agent.model.error_observed")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if !recs[0].Timestamp().Equal(occurred) {
		t.Errorf("timestamp = %v, want %v", recs[0].Timestamp(), occurred)
	}
	attrs := recordAttrs(recs[0])
	if got := attrs["gen_ai.agent.name"].AsString(); got != "assistant" {
		t.Errorf("agent.name = %q", got)
	}
}

func TestEmitAgentOccurrenceInvalidKind(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	in.EmitAgentOccurrence(context.Background(), otelgenai.AgentOccurrence{
		Kind:      "made.up.event",
		Operation: "generate_content",
	})
	if n := len(rec.Records()); n != 0 {
		t.Fatalf("records = %d, want 0 for invalid kind", n)
	}
}

func TestNoEventWithoutProvider(t *testing.T) {
	// Global no-op provider: no events.
	in, err := otelgenai.New(
		otelgenai.WithContentProjector(passthroughProjector),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := in.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: []otelgenai.Message{{Role: "user", Content: "hi"}},
	})
	if !content.Valid() {
		t.Fatal("projection should be valid even without a logger")
	}
	in.EmitInferenceDetails(context.Background(), otelgenai.InferenceDetails{
		Operation:     "generate_content",
		Provider:      "openai",
		InputMessages: content,
	})
	// Nothing to assert directly against a noop; the contract is that
	// Emit does not panic and produces no records.
}

func TestGlobalLoggerProviderFallback(t *testing.T) {
	// Configured global SDK provider receives events without an
	// explicit WithLoggerProvider.
	exporter := &testLogExporter{}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	global.SetLoggerProvider(lp)
	defer func() {
		global.SetLoggerProvider(nooplog.NewLoggerProvider())
		lp.Shutdown(context.Background())
	}()

	in, err := otelgenai.New(otelgenai.WithContentProjector(passthroughProjector))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	in.EmitInferenceDetails(context.Background(), otelgenai.InferenceDetails{
		Operation: "generate_content",
		Provider:  "openai",
		InputMessages: in.ProjectContent(otelgenai.ContentValue{
			Kind:     otelgenai.ContentKindInputMessages,
			Messages: []otelgenai.Message{{Role: "user", Content: "hi"}},
		}),
	})
	if n := len(exporter.records); n != 1 {
		t.Fatalf("exported records = %d, want 1 via global provider", n)
	}
}

func TestExplicitNilLoggerProviderRejected(t *testing.T) {
	_, err := otelgenai.New(otelgenai.WithLoggerProvider(nil))
	if err == nil {
		t.Fatal("New should reject an explicit nil logger provider")
	}
}

func TestEmitEventZeroOccurredAt(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	before := time.Now()
	in.EmitAgentOccurrence(context.Background(), otelgenai.AgentOccurrence{
		Kind:      otelgenai.OccurrenceContinuedAfterError,
		Operation: "generate_content",
	})
	after := time.Now()
	recs := rec.RecordsWithEventName("otelgenai.agent.continued_after_error")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	ts := recs[0].Timestamp()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not captured at emitter entry", ts)
	}
}

func TestSpanConversationID(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	ctx := otelgenai.WithConversationID(context.Background(), "sess-1")
	ctx, op := in.StartInference(ctx, otelgenai.Request{
		Operation: "generate_content",
		Provider:  "openai",
		Model:     "gpt-4",
	})
	op.End(otelgenai.Response{}, nil)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	found := false
	for _, a := range spans[0].Attributes() {
		if a.Key == "gen_ai.conversation.id" && a.Value.AsString() == "sess-1" {
			found = true
		}
	}
	if !found {
		t.Error("span missing gen_ai.conversation.id")
	}
}

func TestProjectContentInvalidWithoutProjector(t *testing.T) {
	in, err := otelgenai.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := in.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: []otelgenai.Message{{Role: "user"}},
	})
	if p.Valid() {
		t.Error("projection without projector must be invalid")
	}
}

func TestProjectContentRespectsLimit(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in, err := otelgenai.New(
		otelgenai.WithTracerProvider(rec.TracerProvider()),
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
		otelgenai.WithContentProjector(passthroughProjector),
		otelgenai.WithProjectionLimit(16),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := in.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: []otelgenai.Message{{Role: "user", Content: "a very long prompt that exceeds the tiny limit"}},
	})
	if p.Valid() {
		t.Error("oversized projection must be invalid")
	}
}

func TestProjectorErrorProducesNoContent(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in, err := otelgenai.New(
		otelgenai.WithLoggerProvider(rec.LoggerProvider()),
		otelgenai.WithContentProjector(func(otelgenai.ContentValue) (otelgenai.ContentValue, error) {
			return otelgenai.ContentValue{}, errors.New("boom")
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := in.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: []otelgenai.Message{{Role: "user", Content: "secret"}},
	})
	if p.Valid() {
		t.Error("projector error must produce invalid content")
	}
}

// testLogExporter collects records for global-provider tests.
type testLogExporter struct {
	records []sdklog.Record
}

func (e *testLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}
func (e *testLogExporter) Shutdown(context.Context) error   { return nil }
func (e *testLogExporter) ForceFlush(context.Context) error { return nil }
