package googlegenai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/codes"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	googlegenai "github.com/hmmftg/otelgenai-go/instrumentation/google-genai"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func TestGenerateContentStream_NormalExhaustion(t *testing.T) {
	srv := newStreamServer(t, []string{
		`{"candidates":[{"content":{"parts":[{"text":"hel"}],"role":"model"}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"lo"}],"role":"model"},"finishReason":"STOP"}],"modelVersion":"gemini-2.5-flash","responseId":"resp-1","usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`,
	})
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	chunkCount := 0
	for resp, err := range models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		chunkCount++
		_ = resp
	}
	if chunkCount != 2 {
		t.Errorf("expected 2 chunks, got %d", chunkCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "generate_content gemini-2.5-flash" {
		t.Errorf("span name: got %q, want %q", span.Name(), "generate_content gemini-2.5-flash")
	}
	assertAttr(t, span, semconv.AttrGenAIRequestStreaming, true)
	assertAttr(t, span, semconv.AttrGenAIResponseModel, "gemini-2.5-flash")
	assertAttr(t, span, semconv.AttrGenAIResponseID, "resp-1")
	assertAttr(t, span, semconv.AttrGenAIUsageInputTokens, int64(5))
	assertAttr(t, span, semconv.AttrGenAIUsageOutputTokens, int64(2))
	assertSpanStatus(t, span, codes.Ok)
}

func TestGenerateContentStream_EarlyBreak(t *testing.T) {
	srv := newStreamServer(t, []string{
		`{"candidates":[{"content":{"parts":[{"text":"hel"}],"role":"model"}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"lo"}],"role":"model"}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"!"}],"role":"model"},"finishReason":"STOP"}],"modelVersion":"gemini-2.5-flash","usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}`,
	})
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	chunkCount := 0
	for resp, err := range models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		chunkCount++
		_ = resp
		if chunkCount >= 2 {
			break // early break before finish reason
		}
	}
	if chunkCount != 2 {
		t.Errorf("expected 2 chunks before break, got %d", chunkCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// The span should exist (early break still finalizes).
	if spans[0].Name() != "generate_content gemini-2.5-flash" {
		t.Errorf("span name: got %q", spans[0].Name())
	}
}

func TestGenerateContentStream_ConstructedButNeverRanged(t *testing.T) {
	srv := newStreamServer(t, []string{
		`{"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"},"finishReason":"STOP"}],"modelVersion":"gemini-2.5-flash","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
	})
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	// Construct the sequence but never range it.
	_ = models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans for unconsumed sequence, got %d", len(spans))
	}
}

func TestGenerateContentStream_MetadataOnlyChunks(t *testing.T) {
	srv := newStreamServer(t, []string{
		// First chunk: metadata only (no text in parts).
		`{"candidates":[{"content":{"parts":[],"role":"model"}}]}`,
		// Second chunk: actual text output.
		`{"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"},"finishReason":"STOP"}],"modelVersion":"gemini-2.5-flash","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
	})
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	chunkCount := 0
	for range models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil) {
		chunkCount++
	}
	if chunkCount != 2 {
		t.Errorf("expected 2 chunks, got %d", chunkCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestGenerateContentStream_ProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error": {"code": 500, "message": "internal error"}}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	var sawError bool
	for _, err := range models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil) {
		if err != nil {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Fatal("expected stream error from 500 response")
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	assertSpanStatus(t, span, codes.Error)
	// Verify raw error message is NOT in the span.
	for _, attr := range span.Attributes() {
		if attr.Value.AsString() == "internal error" {
			t.Error("raw error message leaked into span")
		}
	}
}

func TestGenerateContentStream_BreakAfterFinish(t *testing.T) {
	srv := newStreamServer(t, []string{
		`{"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"},"finishReason":"STOP"}],"modelVersion":"gemini-2.5-flash","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
		`{"candidates":[{"content":{"parts":[{"text":"extra"}],"role":"model"}}]}`,
	})
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	chunkCount := 0
	for resp, err := range models.GenerateContentStream(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		chunkCount++
		// Break after the first chunk which contains the finish reason.
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0] != nil && string(resp.Candidates[0].FinishReason) == "STOP" {
			break
		}
	}
	if chunkCount != 1 {
		t.Errorf("expected 1 chunk before break, got %d", chunkCount)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestHasGeneratedOutput(t *testing.T) {
	tests := []struct {
		name string
		resp *genai.GenerateContentResponse
		want bool
	}{
		{"nil response", nil, false},
		{"no candidates", &genai.GenerateContentResponse{}, false},
		{"nil candidate", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{nil}}, false},
		{"nil content", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{}}}, false},
		{"empty parts", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{}}}}, false},
		{"nil part", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{nil}}}}}, false},
		{"empty text", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: ""}}}}}}, false},
		{"non-empty text", &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}}}}}, true},
		{"metadata only", &genai.GenerateContentResponse{ModelVersion: "gemini-2.5-flash", UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := googlegenai.HasGeneratedOutput(tt.resp)
			if got != tt.want {
				t.Errorf("HasGeneratedOutput(%s): got %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// newStreamServer creates a test server that responds with SSE-style
// JSON chunks for Google GenAI streaming. The SDK expects lines in the
// format "data: {json}\n\n".
func newStreamServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
	}))
}

// Ensure otelgenai is used (for potential future helpers).
var _ = otelgenai.Request{}
