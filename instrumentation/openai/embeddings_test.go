package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/hmmftg/otelgenai-go/instrumentation/openai"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func TestEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object": "list",
			"data": [{"object": "embedding", "index": 0, "embedding": [0.1, 0.2]}],
			"model": "text-embedding-3-small",
			"usage": {"prompt_tokens": 5, "total_tokens": 5}
		}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client := oai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	emb := openai.NewEmbeddings(&client, instr)

	resp, err := emb.New(context.Background(), oai.EmbeddingNewParams{
		Model: oai.EmbeddingModelTextEmbedding3Small,
	})
	if err != nil {
		t.Fatalf("embeddings.New: %v", err)
	}
	if resp.Model != "text-embedding-3-small" {
		t.Fatalf("expected model text-embedding-3-small, got %s", resp.Model)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "embeddings text-embedding-3-small" {
		t.Errorf("span name: got %q, want %q", span.Name(), "embeddings text-embedding-3-small")
	}
	foundInputTokens := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIUsageInputTokens && attr.Value.AsInt64() == 5 {
			foundInputTokens = true
		}
	}
	if !foundInputTokens {
		t.Error("expected gen_ai.usage.input_tokens=5")
	}
}
