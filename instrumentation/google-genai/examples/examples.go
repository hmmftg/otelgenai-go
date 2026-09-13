// Package examples demonstrates instrumenting Google GenAI SDK calls
// with otelgenai-go. These examples do not make real API calls and are
// compiled but not run by default.
package examples

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/google-genai"
)

// ExampleGenerateContent demonstrates wrapping a buffered Google GenAI
// GenerateContent call with GenAI instrumentation.
func ExampleGenerateContent() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  "your-api-key",
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(err)
	}

	models := googlegenai.NewModels(client, instr)

	resp, err := models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello!"}}},
	}, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("model version: %s\n", resp.ModelVersion)
}

// ExampleGenerateContentStream demonstrates wrapping a streaming Google
// GenAI GenerateContentStream call with GenAI instrumentation.
func ExampleGenerateContentStream() {
	instr, _ := otelgenai.New()
	client, _ := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  "your-api-key",
		Backend: genai.BackendGeminiAPI,
	})

	models := googlegenai.NewModels(client, instr)

	ctx := context.Background()
	for resp, err := range models.GenerateContentStream(ctx, "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Tell me a joke"}}},
	}, nil) {
		if err != nil {
			fmt.Printf("stream error: %v\n", err)
			return
		}
		fmt.Print(resp.Text())
	}
}

// ExampleVertexAI demonstrates that the same wrapper works with the
// Vertex AI backend, automatically attributing telemetry to
// gen_ai.system=gcp.vertex_ai.
func ExampleVertexAI() {
	instr, _ := otelgenai.New()
	client, _ := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  "your-project",
		Location: "us-central1",
	})

	models := googlegenai.NewModels(client, instr)

	_, err := models.GenerateContent(context.Background(), "gemini-2.5-flash", []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello!"}}},
	}, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
}
