package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/openai"
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

func TestChatCompletions_Buffered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1700000000,
			"model": "gpt-4o",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "hello"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
		}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := oai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	chat := openai.NewChatCompletions(&client, instr)

	resp, err := chat.New(context.Background(), oai.ChatCompletionNewParams{
		Model: oai.ChatModelGPT4o,
	})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	if resp.ID != "chatcmpl-test" {
		t.Fatalf("expected id chatcmpl-test, got %s", resp.ID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "chat gpt-4o" {
		t.Errorf("span name: got %q, want %q", span.Name(), "chat gpt-4o")
	}
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIResponseID && attr.Value.AsString() != "chatcmpl-test" {
			t.Errorf("response id: got %q, want chatcmpl-test", attr.Value.AsString())
		}
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() != 10 {
			t.Errorf("input tokens: got %d, want 10", attr.Value.AsInt64())
		}
		if string(attr.Key) == semconv.AttrGenAIUsageOutputTokens && attr.Value.AsInt64() != 20 {
			t.Errorf("output tokens: got %d, want 20", attr.Value.AsInt64())
		}
	}
}

func TestChatCompletions_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hel\"}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n")
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
	chat := openai.NewChatCompletions(&client, instr)

	stream := chat.NewStreaming(context.Background(), oai.ChatCompletionNewParams{
		Model: oai.ChatModelGPT4o,
	})
	chunkCount := 0
	for stream.Next() {
		chunkCount++
	}
	if stream.Err() != nil {
		t.Fatalf("stream.Err: %v", stream.Err())
	}
	if chunkCount != 2 {
		t.Errorf("expected 2 chunks, got %d", chunkCount)
	}
	stream.Close()

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "chat gpt-4o" {
		t.Errorf("span name: got %q, want %q", span.Name(), "chat gpt-4o")
	}
	// Verify usage was accumulated from the last chunk.
	foundUsage := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() == 5 {
			foundUsage = true
		}
	}
	if !foundUsage {
		t.Error("expected gen_ai.usage.input_tokens=5 from accumulated stream usage")
	}
}

func TestChatCompletions_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad request","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := oai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	chat := openai.NewChatCompletions(&client, instr)

	_, err := chat.New(context.Background(), oai.ChatCompletionNewParams{
		Model: oai.ChatModelGPT4o,
	})
	if err == nil {
		t.Fatal("expected error from 400 response")
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrErrorType {
			if attr.Value.AsString() == "some secret error message with PII" {
				t.Error("raw error message leaked into span")
			}
		}
	}
}
