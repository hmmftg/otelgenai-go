// Package mcp provides MCP (Model Context Protocol) instrumentation
// using the library's existing INTERNAL GenAI operation model.
//
// v0.3 instruments three MCP operations as INTERNAL spans:
//   - tools/call     → StartTool (execute_tool {name})
//   - resources/read → StartInternalOperation (resources/read)
//   - prompts/get    → StartInternalOperation (prompts/get)
//
// This is repository semantic mapping, not canonical MCP client/server
// RPC span mapping. Canonical MCP CLIENT/SERVER RPC spans are deferred
// until upstream MCP semantic conventions settle around the 2026-07-28
// protocol (see open-telemetry/semantic-conventions-genai#437).
//
// The adapter uses the official github.com/modelcontextprotocol/go-sdk
// middleware API:
//   - Client: AddSendingMiddleware
//   - Server: AddReceivingMiddleware
//
// Trace context is propagated via MCP params._meta using the application's
// configured global TextMapPropagator. The client middleware starts the
// operation first, then injects the span-enriched context into a cloned
// _meta map. The server middleware extracts the remote parent from
// _meta before starting its operation.
//
// Non-instrumented methods pass through completely unchanged: no span
// creation, no context modification, no _meta injection.
//
// Safety: tool arguments, resource contents, and prompt messages are
// never captured. Only metadata (names, URIs, method names) is recorded.
package mcp
