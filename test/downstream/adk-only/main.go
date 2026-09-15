// This module verifies that a consumer depending only on the ADK
// adapter module can build and use it without pulling in the OpenAI,
// Anthropic, or other provider SDKs directly.
package main

import (
	"fmt"

	"github.com/hmmftg/otelgenai-go"
	adkadapter "github.com/hmmftg/otelgenai-go/instrumentation/adk"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}

	// Verify that the ADK plugin can be constructed.
	plugin, err := adkadapter.New(instr)
	if err != nil {
		panic(err)
	}
	if plugin == nil {
		panic("adkadapter.New returned nil plugin")
	}

	fmt.Println("adk-only consumer built successfully")
}
