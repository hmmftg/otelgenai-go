package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"

	"github.com/hmmftg/otelgenai-go/instrumentation/openai"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func TestResponses_Buffered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "resp-test",
			"object": "response",
			"created_at": 1700000000,
			"status": "completed",
			"model": "gpt-4o",
			"usage": {"input_tokens": 8, "output_tokens": 12, "total_tokens": 20, "input_tokens_details": {"cached_tokens": 0}, "output_tokens_details": {"reasoning_tokens": 0}}
		}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := oai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	svc := openai.NewResponses(&client, instr)

	resp, err := svc.New(context.Background(), responses.ResponseNewParams{
		Model: oai.ChatModelGPT4o,
	})
	if err != nil {
		t.Fatalf("responses.New: %v", err)
	}
	if resp.ID != "resp-test" {
		t.Fatalf("expected id resp-test, got %s", resp.ID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Responses API uses generate_content operation.
	if span.Name() != "generate_content gpt-4o" {
		t.Errorf("span name: got %q, want %q", span.Name(), "generate_content gpt-4o")
	}
	foundInputTokens := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() == 8 {
			foundInputTokens = true
		}
	}
	if !foundInputTokens {
		t.Error("expected gen_ai.usage.input_tokens=8")
	}
}

func TestResponses_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// Non-output event (created) - should NOT be timed as output chunk.
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-stream\",\"status\":\"in_progress\",\"model\":\"gpt-4o\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Output text delta - should be timed.
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Another output text delta.
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Completed event with usage.
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-stream\",\"status\":\"completed\",\"model\":\"gpt-4o\",\"usage\":{\"input_tokens\":7,\"output_tokens\":3,\"total_tokens\":10,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens_details\":{\"reasoning_tokens\":0}}}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := oai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	svc := openai.NewResponses(&client, instr)

	stream := svc.NewStreaming(context.Background(), responses.ResponseNewParams{
		Model: oai.ChatModelGPT4o,
	})
	eventCount := 0
	for stream.Next() {
		eventCount++
	}
	if stream.Err() != nil {
		t.Fatalf("stream.Err: %v", stream.Err())
	}
	stream.Close()

	if eventCount != 4 {
		t.Errorf("expected 4 events, got %d", eventCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Verify usage was accumulated from response.completed event.
	foundUsage := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() == 7 {
			foundUsage = true
		}
	}
	if !foundUsage {
		t.Error("expected gen_ai.usage.input_tokens=7 from response.completed event")
	}
}
