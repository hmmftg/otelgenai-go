// This module verifies that a consumer depending only on the Google
// GenAI adapter module can build and use it without pulling in the
// OpenAI or Anthropic SDKs.
package main

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/google-genai"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  "dummy",
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(err)
	}
	models := googlegenai.NewModels(client, instr)
	_ = models
	fmt.Println("google-genai-only consumer built successfully")
}
