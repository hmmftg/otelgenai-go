// This module verifies that a consumer depending only on the root
// otelgenai-go module can build and use the core API without pulling in
// any provider SDK.
package main

import (
	"context"
	"fmt"

	"github.com/hmmftg/otelgenai-go"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}
	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation("chat"),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)
	fmt.Println("root-only consumer built successfully")
}
