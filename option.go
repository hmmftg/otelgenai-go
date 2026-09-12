package otelgenai

import (
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Option configures an Instrumenter.
type Option func(*config)

type config struct {
	tracerProvider         trace.TracerProvider
	meterProvider          metric.MeterProvider
	instrumentationName    string
	instrumentationVersion string
	contentProjector       ContentProjector
	projectionLimit        int
	errorClassifier        ErrorClassifier
	diagnosticHandler      safety.DiagnosticHandler
	disabled               bool
}

// defaultConfig returns the safe default configuration. Production
// defaults use the global OTel providers; they never install exporters
// or globals.
func defaultConfig() config {
	return config{
		tracerProvider:         otel.GetTracerProvider(),
		meterProvider:          otel.GetMeterProvider(),
		instrumentationName:    defaultInstrumentationName,
		instrumentationVersion: defaultInstrumentationVersion,
		projectionLimit:        defaultProjectionLimit,
		errorClassifier:        DefaultClassifier,
	}
}

const (
	defaultInstrumentationName    = "otelgenai-go"
	defaultInstrumentationVersion = "0.1.0"
	defaultProjectionLimit        = 4096
)

// WithTracerProvider sets the tracer provider. Defaults to
// otel.GetTracerProvider().
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) { c.tracerProvider = tp }
}

// WithMeterProvider sets the meter provider. Defaults to
// otel.GetMeterProvider().
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *config) { c.meterProvider = mp }
}

// WithInstrumentationVersion sets the instrumentation version reported
// to OTel. Defaults to the library version.
func WithInstrumentationVersion(v string) Option {
	return func(c *config) { c.instrumentationVersion = v }
}

// WithContentProjector enables opt-in content projection. When set,
// the projector is invoked for each enabled content kind and the
// validated, bounded projection is added to the corresponding span
// attribute. When unset (the default), no content is captured.
func WithContentProjector(p ContentProjector) Option {
	return func(c *config) { c.contentProjector = p }
}

// WithProjectionLimit sets the maximum byte size for a projected
// content value. Projections exceeding this limit are dropped with no
// raw fallback. Defaults to 4096 bytes.
func WithProjectionLimit(limit int) Option {
	return func(c *config) { c.projectionLimit = limit }
}

// WithErrorClassifier sets the error classifier used to map provider
// errors to low-cardinality error.type values. Defaults to
// DefaultClassifier.
func WithErrorClassifier(ec ErrorClassifier) Option {
	return func(c *config) { c.errorClassifier = ec }
}

// WithDiagnosticHandler sets a handler invoked when the library drops
// a projection or recovers a callback panic. The handler receives only
// a stage name and low-cardinality reason code, never rejected content
// or raw errors.
func WithDiagnosticHandler(h safety.DiagnosticHandler) Option {
	return func(c *config) { c.diagnosticHandler = h }
}

// Disabled disables all telemetry emission. When set, the Instrumenter
// returns no-op operations that do not create spans or record metrics.
// This is useful for tests, benchmarks, and conditional instrumentation.
func Disabled() Option {
	return func(c *config) { c.disabled = true }
}
