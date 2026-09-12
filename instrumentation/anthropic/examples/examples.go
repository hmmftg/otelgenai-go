// Package examples demonstrates instrumenting Anthropic SDK calls with
// otelgenai-go. These examples do not make real API calls and are
// compiled but not run by default.
package examples

import (
	"context"
	"fmt"

	anth "github.com/anthropics/anthropic-sdk-go"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/anthropic"
)

// ExampleMessages demonstrates wrapping a Messages call with GenAI
// instrumentation.
func ExampleMessages() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}

	client := anth.NewClient()

	messages := anthropic.NewMessages(&client, instr)

	ctx := context.Background()
	resp, err := messages.New(ctx, anth.MessageNewParams{
		Model:     anth.ModelClaudeSonnet5,
		MaxTokens: 1024,
		Messages: []anth.MessageParam{
			anth.NewUserMessage(anth.NewTextBlock("Hello, Claude!")),
		},
	})
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("response model: %s\n", resp.Model)
}
