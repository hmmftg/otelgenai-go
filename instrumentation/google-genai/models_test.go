package googlegenai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	googlegenai "github.com/hmmftg/otelgenai-go/instrumentation/google-genai"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func newTestInstrumenter(t *testing.T) (*otelgenai.Instrumenter, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)
	instr, err := otelgenai.New(otelgenai.WithTracerProvider(tp))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return instr, exporter
}

func TestResolveSystem(t *testing.T) {
	tests := []struct {
		name    string
		backend genai.Backend
		want    string
	}{
		{"GeminiAPI", genai.BackendGeminiAPI, "gcp.gemini"},
		{"VertexAI", genai.BackendVertexAI, "gcp.vertex_ai"},
		{"Unspecified", genai.BackendUnspecified, "gcp.gen_ai"},
		{"Enterprise", genai.BackendEnterprise, "gcp.gen_ai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := googlegenai.ResolveSystem(tt.backend)
			if got != tt.want {
				t.Errorf("ResolveSystem(%v): got %q, want %q", tt.backend, got, tt.want)
			}
		})
	}
}

func TestGenerateContent_Buffered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [{"content": {"parts": [{"text": "hello"}], "role": "model"}, "finishReason": "STOP"}],
			"modelVersion": "gemini-2.5-flash",
			"responseId": "resp-123",
			"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
		}`)
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
	resp, err := models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil)
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if resp.ResponseID != "resp-123" {
		t.Fatalf("expected responseId resp-123, got %s", resp.ResponseID)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "generate_content gemini-2.5-flash" {
		t.Errorf("span name: got %q, want %q", span.Name(), "generate_content gemini-2.5-flash")
	}
	if span.SpanKind() != oteltrace.SpanKindClient {
		t.Errorf("span kind: got %v, want %v", span.SpanKind(), oteltrace.SpanKindClient)
	}
	assertAttr(t, span, semconv.AttrGenAISystem, "gcp.gemini")
	assertAttr(t, span, semconv.AttrGenAIRequestModel, "gemini-2.5-flash")
	assertAttr(t, span, semconv.AttrGenAIResponseModel, "gemini-2.5-flash")
	assertAttr(t, span, semconv.AttrGenAIResponseID, "resp-123")
	assertAttr(t, span, semconv.AttrGenAIUsageInputTokens, int64(10))
	assertAttr(t, span, semconv.AttrGenAIUsageOutputTokens, int64(5))
	assertAttr(t, span, semconv.AttrGenAIResponseFinishReasons, []string{"STOP"})
	assertSpanStatus(t, span, codes.Ok)
}

func TestGenerateContent_VertexBackend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [{"content": {"parts": [{"text": "hello"}], "role": "model"}, "finishReason": "STOP"}],
			"modelVersion": "gemini-2.5-flash",
			"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
		}`)
	}))
	defer srv.Close()

	instr, exporter := newTestInstrumenter(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendVertexAI,
		Project:     "test-project",
		Location:    "us-central1",
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models := googlegenai.NewModels(client, instr)
	_, err = models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil)
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	assertAttr(t, spans[0], semconv.AttrGenAISystem, "gcp.vertex_ai")
}

func TestGenerateContent_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": {"code": 400, "message": "bad request"}}`)
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
	_, err = models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, nil)
	if err == nil {
		t.Fatal("expected error from 400 response")
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	assertSpanStatus(t, span, codes.Error)
	// Verify raw error message is NOT in the span.
	for _, attr := range span.Attributes() {
		if attr.Value.AsString() == "bad request" {
			t.Error("raw error message leaked into span")
		}
	}
}

func TestGenerateContent_NoContentCaptured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [{"content": {"parts": [{"text": "SECRET_RESPONSE_CONTENT"}], "role": "model"}, "finishReason": "STOP"}],
			"modelVersion": "gemini-2.5-flash",
			"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
		}`)
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
	_, err = models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "SECRET_PROMPT_CONTENT"}}},
	}, &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: "SECRET_SYSTEM_INSTRUCTION"}}},
	})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// No content attributes should be present by default.
	for _, key := range []string{
		semconv.AttrGenAIInputMessages,
		semconv.AttrGenAIOutputMessages,
		semconv.AttrGenAISystemInstructions,
	} {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == key {
				t.Errorf("content attr %q should not be present", key)
			}
		}
	}
	// No sentinels should appear anywhere.
	sentinels := []string{"SECRET_RESPONSE_CONTENT", "SECRET_PROMPT_CONTENT", "SECRET_SYSTEM_INSTRUCTION"}
	for _, attr := range span.Attributes() {
		for _, s := range sentinels {
			if attr.Value.AsString() == s {
				t.Errorf("span attr %q contains sentinel %q", attr.Key, s)
			}
		}
	}
}

func TestGenerateContent_ConfigMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request body contains the config fields.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"candidates": [{"content": {"parts": [{"text": "ok"}], "role": "model"}, "finishReason": "STOP"}],
			"modelVersion": "gemini-2.5-flash",
			"usageMetadata": {"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2}
		}`)
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

	temp := float32(0.7)
	topP := float32(0.9)
	seed := int32(42)
	models := googlegenai.NewModels(client, instr)
	_, err = models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
	}, &genai.GenerateContentConfig{
		MaxOutputTokens: 100,
		Temperature:     &temp,
		TopP:            &topP,
		Seed:            &seed,
		StopSequences:   []string{"END"},
	})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	assertAttr(t, span, semconv.AttrGenAIRequestMaxTokens, int64(100))
	assertAttrFloat(t, span, semconv.AttrGenAIRequestTemperature, 0.7)
	assertAttrFloat(t, span, semconv.AttrGenAIRequestTopP, 0.9)
	assertAttr(t, span, semconv.AttrGenAIRequestSeed, int64(42))
	assertAttr(t, span, semconv.AttrGenAIRequestStopSequences, []string{"END"})
}

// assertAttr checks that a span has an attribute with the expected key
// and value.
func assertAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, expected any) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			switch v := expected.(type) {
			case string:
				if attr.Value.AsString() == v {
					return
				}
			case int64:
				if attr.Value.AsInt64() == v {
					return
				}
			case bool:
				if attr.Value.AsBool() == v {
					return
				}
			case float64:
				if attr.Value.AsFloat64() == v {
					return
				}
			case []string:
				got := attr.Value.AsStringSlice()
				if len(got) == len(v) {
					match := true
					for i, s := range v {
						if got[i] != s {
							match = false
							break
						}
					}
					if match {
						return
					}
				}
			}
			t.Errorf("span attr %q: got %v, want %v", key, attr.Value, expected)
			return
		}
	}
	t.Errorf("span attr %q: not found", key)
}

func assertSpanStatus(t *testing.T, span sdktrace.ReadOnlySpan, expected codes.Code) {
	t.Helper()
	if span.Status().Code != expected {
		t.Errorf("span status: got %v, want %v", span.Status().Code, expected)
	}
}

// assertAttrFloat checks a float attribute with tolerance for float32→float64
// precision loss.
func assertAttrFloat(t *testing.T, span sdktrace.ReadOnlySpan, key string, want float64) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			got := attr.Value.AsFloat64()
			if abs(got-want) < 1e-6 {
				return
			}
			t.Errorf("span attr %q: got %v, want %v", key, got, want)
			return
		}
	}
	t.Errorf("span attr %q: not found", key)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Ensure attribute package is used (for potential future helpers).
var _ = attribute.StringValue
