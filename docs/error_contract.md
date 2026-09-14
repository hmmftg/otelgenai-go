# Cross-Adapter Error Classification Contract

This document specifies the cross-adapter error classification contract for
`otelgenai-go`. All provider adapters (OpenAI, Anthropic, Google GenAI, MCP)
MUST produce the same `error.type` attribute value for the same class of
error, ensuring that downstream dashboards and alerts can rely on a stable
taxonomy regardless of the provider.

## Contract

| Error scenario | `error.type` | Classification path |
|---|---|---|
| No error | `""` (none) | `ErrorTypeNone` |
| `context.Canceled` | `cancelled` | `classifyContextError` |
| `context.DeadlineExceeded` | `timeout` | `classifyContextError` |
| Early stream close | `stream.closed` | `classifyContextError` (via `errStreamClosedEarly`) |
| Unclassified error | `unknown` | `DefaultClassifier` fallback |

## Principles

1. **Low cardinality**: `error.type` values are a fixed enumeration. Raw
   error messages, HTTP status codes, and provider-specific error codes are
   never recorded in span attributes.

2. **Consistency across adapters**: The same error class produces the same
   `error.type` regardless of which provider adapter is in use. The
   conformance test exercises the same core classification path
   (`classifyContextError` + `DefaultClassifier`) rather than manufacturing
   provider-native errors that merely resemble the categories.

3. **No silent changes**: Adapters that already have provider-specific
   classifications (e.g., `provider.4xx`, `provider.5xx`, `transport`) retain
   those classifications. The conformance test verifies that the core
   categories (`cancelled`, `timeout`, `stream.closed`, `unknown`) are
   consistent; it does not force adapters to drop existing
   provider-specific classifications.

4. **Extensibility**: New error types can be added to the `ErrorType`
   enumeration in `errors.go`. The contract guarantees that the existing
   core categories remain stable.

## Conformance Testing

The conformance test (`error_contract_test.go`) verifies that all operation
types (inference, tool, internal) produce the same `error.type` for the same
error class. It exercises the core classification path directly rather than
attempting to manufacture provider-native errors that happen to look
equivalent.
