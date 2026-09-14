// Package safety provides panic-isolated execution of library-owned
// extension callbacks such as content projectors, error classifiers,
// and diagnostic handlers. The library never lets a panicking callback
// crash the application or fall back to raw, unmasked data.
package safety

import "fmt"

// Reason is a low-cardinality diagnostic reason code. It must never
// contain rejected content, raw errors, or other sensitive data.
type Reason string

// Reason values are low-cardinality diagnostic reason codes for each
// failure stage of content projection and callback execution.
const (
	ReasonProjectorPanic    Reason = "projector.panic"
	ReasonProjectorError    Reason = "projector.error"
	ReasonProjectorOversize Reason = "projector.oversize"
	ReasonProjectorInvalid  Reason = "projector.invalid_shape"
	ReasonProjectorBadJSON  Reason = "projector.invalid_json"
	ReasonClassifierPanic   Reason = "classifier.panic"
	ReasonDiagnosticPanic   Reason = "diagnostic.panic"
	ReasonResolverPanic     Reason = "pricing.resolver.panic"
)

// Diagnostic is a typed diagnostic record containing only a stage
// identifier and a low-cardinality reason code. It never includes
// rejected content or raw errors.
type Diagnostic struct {
	Stage  string
	Reason Reason
}

// DiagnosticHandler is invoked when the library drops a projection or
// recovers a callback panic. Implementations must not panic; if they
// do, the panic is recovered and silently dropped.
type DiagnosticHandler func(Diagnostic)

// GuardedCall invokes fn and recovers any panic, converting it to an
// error. The recovered value is never included in the returned error
// message to avoid leaking sensitive data; only a generic message is
// returned.
func GuardedCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("callback panicked")
		}
	}()
	return fn()
}

// GuardedCallValue invokes fn returning a value and error, and recovers
// any panic, converting it to an error with a generic message.
func GuardedCallValue[T any](fn func() (T, error)) (v T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("callback panicked")
		}
	}()
	return fn()
}

// GuardedDiagnostic invokes a diagnostic handler, recovering any panic
// and silently dropping it. The diagnostic handler must never receive
// sensitive data.
func GuardedDiagnostic(handler DiagnosticHandler, d Diagnostic) {
	if handler == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	handler(d)
}
