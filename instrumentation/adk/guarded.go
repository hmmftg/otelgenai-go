package adk

// guardedCall invokes fn and recovers any panic, returning true when fn
// completed without panicking and false when it panicked. The recovered
// value is never retained or exposed, matching the core safety contract
// that callback panics must not leak sensitive data.
//
// This is an ADK-local helper so the adapter does not depend on the
// internal/safety package. It intentionally returns only a boolean
// outcome; the caller reports a bounded diagnostic through the
// constrained adapter API (ReportInstrumentationFailure) when needed.
func guardedCall(fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	fn()
	return true
}
