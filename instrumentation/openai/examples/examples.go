// Package examples demonstrates instrumenting OpenAI SDK calls with
// otelgenai-go. These examples do not make real API calls and are
// compiled but not run by default.
package examples

import (
	"context"
	"fmt"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/openai"
)

// ExampleChat demonstrates wrapping a Chat Completions call with
// GenAI instrumentation.
func ExampleChat() {
	// Create the instrumenter (uses global OTel providers by default).
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}

	// Create the official OpenAI client.
	client := oai.NewClient()

	// Wrap the Chat Completions service.
	chat := openai.NewChatCompletions(&client, instr)

	// Make an instrumented call. The span encloses all SDK retries.
	ctx := context.Background()
	resp, err := chat.New(ctx, oai.ChatCompletionNewParams{
		Model: oai.ChatModelGPT4o,
		Messages: []oai.ChatCompletionMessageParamUnion{
			oai.UserMessage("Hello!"),
		},
	})
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("response model: %s\n", resp.Model)
}

// ExampleStreaming demonstrates wrapping a streaming Chat Completions
// call with GenAI instrumentation.
func ExampleStreaming() {
	instr, _ := otelgenai.New()
	client := oai.NewClient(option.WithAPIKey("dummy"))
	chat := openai.NewChatCompletions(&client, instr)

	ctx := context.Background()
	stream := chat.NewStreaming(ctx, oai.ChatCompletionNewParams{
		Model: oai.ChatModelGPT4o,
		Messages: []oai.ChatCompletionMessageParamUnion{
			oai.UserMessage("Tell me a joke"),
		},
	})
	defer func() { _ = stream.Close() }()

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}
}

// ExampleOpenAICompatible demonstrates instrumenting an
// OpenAI-compatible provider (e.g. Ollama, vLLM, Groq) using the
// official OpenAI SDK with a custom base URL and the WithSystem option.
func ExampleOpenAICompatible() {
	instr, _ := otelgenai.New()

	// Create the official OpenAI client pointing at the compatible
	// endpoint.
	client := oai.NewClient(
		option.WithBaseURL("http://localhost:11434/v1"),
		option.WithAPIKey("dummy"),
	)

	// Wrap with instrumentation, overriding gen_ai.system so the
	// telemetry identifies the provider correctly.
	chat := openai.NewChatCompletions(&client, instr, openai.WithSystem("ollama"))

	ctx := context.Background()
	resp, err := chat.New(ctx, oai.ChatCompletionNewParams{
		Model: "llama3.2",
		Messages: []oai.ChatCompletionMessageParamUnion{
			oai.UserMessage("Hello!"),
		},
	})
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("response model: %s\n", resp.Model)
}
