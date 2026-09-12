package anthropic_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	anth "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/anthropic"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func newTestInstrumenter(t *testing.T) (*otelgenai.Instrumenter, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
	)
	instr, err := otelgenai.New(otelgenai.WithTracerProvider(tp))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return instr, exporter
}

func TestMessages_Buffered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "msg-test",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-5",
			"content": [{"type": "text", "text": "hello"}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}
		}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := anth.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	msgs := anthropic.NewMessages(&client, instr)

	resp, err := msgs.New(context.Background(), anth.MessageNewParams{
		Model:     anth.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatalf("messages.New: %v", err)
	}
	if resp.ID != "msg-test" {
		t.Fatalf("expected id msg-test, got %s", resp.ID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "chat claude-sonnet-4-5" {
		t.Errorf("span name: got %q, want %q", span.Name(), "chat claude-sonnet-4-5")
	}
	foundInputTokens := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() == 10 {
			foundInputTokens = true
		}
	}
	if !foundInputTokens {
		t.Error("expected gen_ai.usage.input_tokens=10")
	}
}

func TestMessages_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// message_start - should NOT be timed as output chunk.
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-stream\",\"model\":\"claude-sonnet-4-5\",\"role\":\"assistant\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":8,\"output_tokens\":1,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// content_block_start - should NOT be timed as output chunk.
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// content_block_delta - SHOULD be timed as output chunk.
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hel\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Another content_block_delta - SHOULD be timed.
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// content_block_stop - should NOT be timed.
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// message_delta with cumulative usage - should NOT be timed.
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":8,\"output_tokens\":3,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// message_stop.
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := anth.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	msgs := anthropic.NewMessages(&client, instr)

	stream := msgs.NewStreaming(context.Background(), anth.MessageNewParams{
		Model:     anth.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
	})
	eventCount := 0
	for stream.Next() {
		eventCount++
	}
	if stream.Err() != nil {
		t.Fatalf("stream.Err: %v", stream.Err())
	}
	stream.Close()

	// 6 events: message_start, content_block_start, 2x content_block_delta, content_block_stop, message_delta, message_stop
	if eventCount != 7 {
		t.Errorf("expected 7 events, got %d", eventCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Verify usage was accumulated from message_delta event (cumulative, last value).
	foundUsage := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageOutputTokens && attr.Value.AsInt64() == 3 {
			foundUsage = true
		}
	}
	if !foundUsage {
		t.Error("expected gen_ai.usage.output_tokens=3 from message_delta cumulative usage")
	}
}

func TestMessages_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := anth.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	msgs := anthropic.NewMessages(&client, instr)

	_, err := msgs.New(context.Background(), anth.MessageNewParams{
		Model:     anth.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
	})
	if err == nil {
		t.Fatal("expected error from 400 response")
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Verify raw error message did NOT leak into span attributes.
	for _, attr := range span.Attributes() {
		if attr.Value.AsString() == "bad request" {
			t.Errorf("raw error message leaked into span attr %q", attr.Key)
		}
	}
}
