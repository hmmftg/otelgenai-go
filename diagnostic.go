package otelgenai

import "github.com/hmmftg/otelgenai-go/internal/safety"

// DiagnosticReason is a stable, low-cardinality reason code reported to
// configured diagnostic handlers. It is the external observation
// contract: consumers may switch on these values. It never contains
// rejected content, raw errors, or other sensitive data.
//
// DiagnosticReason is distinct from [InstrumentationFailure]:
// InstrumentationFailure is the bounded input adapters use to report
// internal failures; DiagnosticReason is the observation delivered to
// diagnostic handlers.
type DiagnosticReason = safety.Reason

// Diagnostic reason values. The set is fixed; new values are added
// deliberately and never dynamically.
const (
	// DiagnosticReasonProjectorPanic indicates a content projector
	// panicked. The projection is dropped with no raw fallback.
	DiagnosticReasonProjectorPanic = safety.ReasonProjectorPanic
	// DiagnosticReasonProjectorError indicates a content projector
	// returned an error. The projection is dropped.
	DiagnosticReasonProjectorError = safety.ReasonProjectorError
	// DiagnosticReasonProjectorOversize indicates a projection exceeded
	// the configured byte limit. It is dropped.
	DiagnosticReasonProjectorOversize = safety.ReasonProjectorOversize
	// DiagnosticReasonProjectorInvalidShape indicates a projection did
	// not match the shape required by its content kind.
	DiagnosticReasonProjectorInvalid = safety.ReasonProjectorInvalid
	// DiagnosticReasonProjectorBadJSON indicates a projection could not
	// be serialized to JSON.
	DiagnosticReasonProjectorBadJSON = safety.ReasonProjectorBadJSON
	// DiagnosticReasonClassifierPanic indicates a configured error
	// classifier panicked. The classification falls back to unknown.
	DiagnosticReasonClassifierPanic = safety.ReasonClassifierPanic
	// DiagnosticReasonDiagnosticPanic indicates a diagnostic handler
	// panicked. The panic is recovered and silently dropped.
	DiagnosticReasonDiagnosticPanic = safety.ReasonDiagnosticPanic
	// DiagnosticReasonPricingResolverPanic indicates a pricing resolver
	// panicked. No cost attribute is emitted.
	DiagnosticReasonPricingResolverPanic = safety.ReasonResolverPanic
	// DiagnosticReasonModelStateConflict indicates a model operation was
	// started while another was already active for the same identity.
	DiagnosticReasonModelStateConflict = safety.ReasonModelStateConflict
	// DiagnosticReasonToolSystemResolverPanic indicates a tool system
	// resolver panicked. The gen_ai.system attribute is omitted.
	DiagnosticReasonToolSystemResolverPanic = safety.ReasonToolSystemResolverPanic
	// DiagnosticReasonInvalidToolSpan indicates a tool lifecycle
	// callback arrived without a valid active execute_tool span.
	DiagnosticReasonInvalidToolSpan = safety.ReasonInvalidToolSpan
	// DiagnosticReasonInvalidMetricValue indicates a metric recording
	// received an invalid value (e.g. negative). The measurement is
	// omitted.
	DiagnosticReasonInvalidMetricValue = safety.ReasonInvalidMetricValue
	// DiagnosticReasonInvalidEvent indicates an event emitter was called
	// with an invalid payload (missing required identity or unknown
	// occurrence kind). The event is not emitted.
	DiagnosticReasonInvalidEvent = safety.ReasonInvalidEvent
	// DiagnosticReasonProviderResolverPanic indicates an inference
	// provider-name resolver panicked. The event is not emitted.
	DiagnosticReasonProviderResolverPanic = safety.ReasonProviderResolverPanic
	// DiagnosticReasonUnknown is reported when an adapter reports an
	// unrecognized InstrumentationFailure value.
	DiagnosticReasonUnknown = safety.ReasonUnknown
)

// Diagnostic is a bounded observation delivered to the configured
// diagnostic handler. It contains only a stage identifier and a
// low-cardinality [DiagnosticReason]; it never includes rejected
// content, raw errors, or panic values.
type Diagnostic = safety.Diagnostic

// DiagnosticHandler is invoked when the library drops a projection,
// recovers a callback panic, or observes an internal instrumentation
// failure. Implementations must not panic; if they do, the panic is
// recovered and silently dropped.
//
// The handler receives only bounded, low-cardinality data. It is the
// application's policy layer for aggregation, deduplication, and rate
// limiting; the library does not deduplicate observations.
type DiagnosticHandler = safety.DiagnosticHandler
