// This module verifies that a consumer depending only on the MCP
// adapter module can build and use it without pulling in the OpenAI,
// Anthropic, or Google GenAI SDKs.
package main

import (
	"context"
	"fmt"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hmmftg/otelgenai-go"
	mcpadapter "github.com/hmmftg/otelgenai-go/instrumentation/mcp"
)

func main() {
	instr, err := otelgenai.New()
	if err != nil {
		panic(err)
	}

	// Verify that the MCP client and server middleware can be constructed.
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	client.AddSendingMiddleware(mcpadapter.ClientMiddleware(instr))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	server.AddReceivingMiddleware(mcpadapter.ServerMiddleware(instr))

	_ = context.Background()
	fmt.Println("mcp-only consumer built successfully")
}
