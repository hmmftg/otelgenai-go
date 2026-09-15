package otelgenai_test

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/testutil"
	"go.opentelemetry.io/otel/attribute"
)

func TestConversationIDAbsent(t *testing.T) {
	id, ok := otelgenai.ConversationIDFromContext(context.Background())
	if ok || id != "" {
		t.Fatalf("absent: got (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestConversationIDNilContext(t *testing.T) {
	//nolint:staticcheck // deliberately testing nil ctx handling
	id, ok := otelgenai.ConversationIDFromContext(nil)
	if ok || id != "" {
		t.Fatalf("nil ctx: got (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestConversationIDSet(t *testing.T) {
	ctx := otelgenai.WithConversationID(context.Background(), "conv-1")
	id, ok := otelgenai.ConversationIDFromContext(ctx)
	if !ok || id != "conv-1" {
		t.Fatalf("set: got (%q, %v), want (conv-1, true)", id, ok)
	}
}

func TestConversationIDEmptyClears(t *testing.T) {
	parent := otelgenai.WithConversationID(context.Background(), "conv-1")
	ctx := otelgenai.WithConversationID(parent, "")
	id, ok := otelgenai.ConversationIDFromContext(ctx)
	if !ok || id != "" {
		t.Fatalf("cleared: got (%q, %v), want (\"\", true)", id, ok)
	}
}

func TestConversationIDChildReSetAfterClear(t *testing.T) {
	parent := otelgenai.WithConversationID(context.Background(), "conv-1")
	cleared := otelgenai.WithConversationID(parent, "")
	reSet := otelgenai.WithConversationID(cleared, "conv-2")
	id, ok := otelgenai.ConversationIDFromContext(reSet)
	if !ok || id != "conv-2" {
		t.Fatalf("re-set: got (%q, %v), want (conv-2, true)", id, ok)
	}
}

func TestConversationIDParentUnaffectedByChildClear(t *testing.T) {
	parent := otelgenai.WithConversationID(context.Background(), "conv-1")
	_ = otelgenai.WithConversationID(parent, "")
	id, ok := otelgenai.ConversationIDFromContext(parent)
	if !ok || id != "conv-1" {
		t.Fatalf("parent: got (%q, %v), want (conv-1, true)", id, ok)
	}
}

func TestClearedConversationIDOmitsSpanAttr(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in, err := otelgenai.New(otelgenai.WithTracerProvider(rec.TracerProvider()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := otelgenai.WithConversationID(context.Background(), "conv-1")
	ctx = otelgenai.WithConversationID(ctx, "")
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
	for _, a := range spans[0].Attributes() {
		if a.Key == "gen_ai.conversation.id" {
			t.Fatalf("cleared conversation ID must not appear on span, got %v", a.Value.AsString())
		}
	}
}

func TestClearedConversationIDOmitsEventAttr(t *testing.T) {
	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())
	in := newEventInstrumenter(t, rec)

	ctx := otelgenai.WithConversationID(context.Background(), "conv-1")
	ctx = otelgenai.WithConversationID(ctx, "")
	in.EmitAgentOccurrence(ctx, otelgenai.AgentOccurrence{
		Kind:      otelgenai.OccurrenceModelErrorObserved,
		Operation: "generate_content",
	})

	recs := rec.RecordsWithEventName("otelgenai.agent.model.error_observed")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	recs[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == "gen_ai.conversation.id" {
			t.Fatalf("cleared conversation ID must not appear on event")
		}
		return true
	})
}
