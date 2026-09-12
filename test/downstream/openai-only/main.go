// This module verifies that a consumer depending only on the OpenAI
// adapter module can build and use it without pulling in the Anthropic
// SDK.
package main

import (
	"context"
	"fmt"

	oai "github.com/openai/openai-go"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/openai"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}
	client := oai.NewClient()
	chat := openai.NewChatCompletions(&client, instr)
	_ = chat
	_ = context.Background()
	fmt.Println("openai-only consumer built successfully")
}
