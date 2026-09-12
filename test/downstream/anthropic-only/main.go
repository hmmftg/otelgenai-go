// This module verifies that a consumer depending only on the Anthropic
// adapter module can build and use it without pulling in the OpenAI SDK.
package main

import (
	"fmt"

	anth "github.com/anthropics/anthropic-sdk-go"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/instrumentation/anthropic"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}
	client := anth.NewClient()
	msgs := anthropic.NewMessages(&client, instr)
	_ = msgs
	fmt.Println("anthropic-only consumer built successfully")
}
