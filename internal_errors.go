package otelgenai

import (
	"context"
	"errors"
)

// classifyContextError returns true if the error is a context cancellation,
// deadline error, or early stream closure, setting the corresponding
// ErrorType.
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
	if errors.Is(err, errStreamClosedEarly) {
		return ErrorTypeStreamClosed, true
	}
	return ErrorTypeNone, false
}
