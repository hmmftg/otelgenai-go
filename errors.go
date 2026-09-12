package otelgenai

// ErrorType is a low-cardinality classification of errors that may occur
// during a GenAI operation. Raw error messages are never recorded in
// telemetry; only this stable classification is used for the error.type
// attribute and span status.
type ErrorType string

const (
	// ErrorTypeNone indicates no error.
	ErrorTypeNone ErrorType = ""

	// ErrorTypeCancelled indicates the operation was cancelled.
	ErrorTypeCancelled ErrorType = "cancelled"

	// ErrorTypeTimeout indicates the operation timed out.
	ErrorTypeTimeout ErrorType = "timeout"

	// ErrorTypeTransport indicates a network/transport-level failure
	// such as connection refused, DNS resolution failure, or TLS error.
	ErrorTypeTransport ErrorType = "transport"

	// ErrorTypeProvider4xx indicates the provider returned an HTTP 4xx
	// client error (e.g. authentication, rate limit, bad request).
	ErrorTypeProvider4xx ErrorType = "provider.4xx"

	// ErrorTypeProvider5xx indicates the provider returned an HTTP 5xx
	// server error.
	ErrorTypeProvider5xx ErrorType = "provider.5xx"

	// ErrorTypeProviderError indicates the provider returned an error
	// response with an unclassified status.
	ErrorTypeProviderError ErrorType = "provider.error"

	// ErrorTypeStreamClosed indicates the stream was closed before a
	// terminal event was received.
	ErrorTypeStreamClosed ErrorType = "stream.closed"

	// ErrorTypeStreamError indicates the stream produced an error event.
	ErrorTypeStreamError ErrorType = "stream.error"

	// ErrorTypeMalformed indicates the provider returned a malformed
	// response that the SDK could not parse.
	ErrorTypeMalformed ErrorType = "malformed"

	// ErrorTypeUnknown indicates an error that does not match any
	// known classification.
	ErrorTypeUnknown ErrorType = "unknown"
)

// ErrorClassifier maps a provider error to a low-cardinality ErrorType.
// The classifier must never include the raw error message in its return
// value. Implementations should inspect HTTP status codes, error types,
// or sentinel values rather than error strings.
type ErrorClassifier func(err error) ErrorType

// DefaultClassifier provides a conservative classification that returns
// ErrorTypeUnknown for errors it cannot specifically identify. It does
// not inspect error strings to avoid leaking sensitive content.
func DefaultClassifier(err error) ErrorType {
	if err == nil {
		return ErrorTypeNone
	}
	if et, ok := classifyContextError(err); ok {
		return et
	}
	return ErrorTypeUnknown
}
