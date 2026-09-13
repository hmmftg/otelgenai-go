package mcp

import (
	"context"

	"github.com/hmmftg/otelgenai-go"
	"go.opentelemetry.io/otel/attribute"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP attribute constants (adapter-local, not in core semconv).
const (
	// AttrMCPMethodName is the MCP method name attribute.
	AttrMCPMethodName = "mcp.method.name"
	// AttrMCPResourceURI is the resource URI attribute for resources/read.
	AttrMCPResourceURI = "mcp.resource.uri"
	// AttrGenAIPromptName is the prompt name attribute for prompts/get.
	AttrGenAIPromptName = "gen_ai.prompt.name"
)

// MCP method name constants.
const (
	MethodToolsCall    = "tools/call"
	MethodResourcesRead = "resources/read"
	MethodPromptsGet    = "prompts/get"
)

// isInstrumented reports whether the given MCP method should be
// instrumented by this adapter.
func isInstrumented(method string) bool {
	switch method {
	case MethodToolsCall, MethodResourcesRead, MethodPromptsGet:
		return true
	default:
		return false
	}
}

// extractTarget extracts the low-cardinality target (tool name, prompt
// name) or resource URI from the request params. For resources/read,
// the URI is returned as the target for attribute purposes (not for
// span names). Returns empty string if the params type is unrecognized.
func extractTarget(method string, req mcp.Request) string {
	if req == nil {
		return ""
	}
	params := req.GetParams()
	if params == nil {
		return ""
	}
	switch method {
	case MethodToolsCall:
		// Client-side: *CallToolParams; Server-side: *CallToolParamsRaw
		if p, ok := params.(*mcp.CallToolParams); ok {
			return p.Name
		}
		if p, ok := params.(*mcp.CallToolParamsRaw); ok {
			return p.Name
		}
	case MethodResourcesRead:
		if p, ok := params.(*mcp.ReadResourceParams); ok {
			return p.URI
		}
	case MethodPromptsGet:
		if p, ok := params.(*mcp.GetPromptParams); ok {
			return p.Name
		}
	}
	return ""
}

// buildAttributes constructs the MCP-specific span attributes for the
// given method and target.
func buildAttributes(method, target string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(AttrMCPMethodName, method),
	}
	switch method {
	case MethodToolsCall:
		if target != "" {
			attrs = append(attrs, attribute.String("gen_ai.tool.name", target))
		}
		attrs = append(attrs, attribute.String("gen_ai.tool.type", "mcp"))
	case MethodResourcesRead:
		if target != "" {
			attrs = append(attrs, attribute.String(AttrMCPResourceURI, target))
		}
	case MethodPromptsGet:
		if target != "" {
			attrs = append(attrs, attribute.String(AttrGenAIPromptName, target))
		}
	}
	return attrs
}

// startOperation starts the appropriate core operation for the given
// MCP method. For tools/call, it uses StartTool (which increments the
// agent's tool-call counter). For resources/read and prompts/get, it
// uses StartInternalOperation (no agent counter increment).
//
// The returned operation interface exposes End(error) matching both
// *otelgenai.ToolOperation and *otelgenai.InternalOperation.
type endable interface {
	End(error)
}

func startOperation(ctx context.Context, instr *otelgenai.Instrumenter, method, target string) (context.Context, endable) {
	attrs := buildAttributes(method, target)
	switch method {
	case MethodToolsCall:
		return instr.StartTool(ctx, otelgenai.ToolRequest{
			Name:  target,
			Type:  "mcp",
			Attrs: attrs,
		})
	default:
		return instr.StartInternalOperation(ctx, method, attrs...)
	}
}
