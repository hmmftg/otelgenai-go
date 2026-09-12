package otelgenai

import (
	"context"
	"errors"
)

// Sentinel errors used by the default classifier. These are defined here
// rather than imported from the standard library to keep the classifier
// testable without coupling to context error identity, which can vary
// when errors are wrapped.
var (
	contextCancelled      = errors.New("context cancelled")
	contextDeadlineExceeded = errors.New("context deadline exceeded")
)

// classifyContextError returns true if the error is a context cancellation
// or deadline error, setting the corresponding ErrorType.
func classifyContextError(err error) (ErrorType, bool) {
	if err == nil {
		return ErrorTypeNone, false
	}
	if errors.Is(err, context.Canceled) {
		return ErrorTypeCancelled, true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeTimeout, true
	}
	return ErrorTypeNone, false
}
