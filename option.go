package otelgenai

import (
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/pricing"
	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Option configures an Instrumenter.
type Option func(*config)

type config struct {
	tracerProvider         trace.TracerProvider
	meterProvider          metric.MeterProvider
	loggerProvider         otellog.LoggerProvider
	loggerProviderSet      bool
	instrumentationName    string
	instrumentationVersion string
	contentProjector       ContentProjector
	projectionLimit        int
	errorClassifier        ErrorClassifier
	diagnosticHandler      safety.DiagnosticHandler
	pricingResolver        pricing.PricingResolver
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
	defaultInstrumentationVersion = Version
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

// WithLoggerProvider sets the logger provider used for correlated events.
// When unset, the global LoggerProvider is resolved at Instrumenter
// construction time; a global no-op provider means no events are emitted.
// An explicitly nil provider is invalid and rejected by NewInstrumenter.
func WithLoggerProvider(lp otellog.LoggerProvider) Option {
	return func(c *config) {
		c.loggerProvider = lp
		c.loggerProviderSet = true
	}
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
// a projection, recovers a callback panic, or observes an internal
// instrumentation failure. The handler receives only a bounded
// [Diagnostic] with a stage name and low-cardinality
// [DiagnosticReason]; it never receives rejected content, raw errors,
// or panic values. The handler is the application's policy layer for
// aggregation, deduplication, and rate limiting.
func WithDiagnosticHandler(h DiagnosticHandler) Option {
	return func(c *config) { c.diagnosticHandler = h }
}

// Disabled disables all telemetry emission. When set, the Instrumenter
// returns no-op operations that do not create spans or record metrics.
// This is useful for tests, benchmarks, and conditional instrumentation.
func Disabled() Option {
	return func(c *config) { c.disabled = true }
}

// WithPricingResolver enables optional estimated-cost derivation from
// token usage. When set, the resolver is called for each inference
// operation to derive an estimated cost, which is recorded as a span
// attribute (gen_ai.usage.estimated_cost, float64, USD). When unset (the
// default), no cost telemetry is emitted.
//
// Cost is estimated, not authoritative billing. Pricing lookup failures,
// unknown models, invalid prices, and inconsistent usage data all
// result in no cost attribute being emitted; the inference operation
// is never affected.
func WithPricingResolver(r pricing.PricingResolver) Option {
	return func(c *config) { c.pricingResolver = r }
}
