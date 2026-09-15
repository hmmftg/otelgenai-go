package otelgenai

import (
	"errors"
	"fmt"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/pricing"
)

// Instrumenter is the framework-neutral entry point for GenAI
// instrumentation. It creates spans and metrics for inference,
// streaming, agent invocation, and tool execution operations.
//
// All telemetry is safe by default: no prompts, responses, tool
// arguments, tool results, credentials, cookies, or raw bodies are
// captured unless an explicit ContentProjector is configured.
type Instrumenter struct {
	tracer     trace.Tracer
	logger     otellog.Logger
	metrics    metrics
	cfg        config
	classifier ErrorClassifier
	diag       safety.DiagnosticHandler
	pricing    pricing.PricingResolver
}

// New creates a new Instrumenter with the given options. All metric
// instruments are created at construction so validation failures
// surface immediately.
func New(opts ...Option) (*Instrumenter, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.errorClassifier == nil {
		return nil, errors.New("otelgenai: error classifier must not be nil")
	}
	if cfg.projectionLimit <= 0 {
		return nil, errors.New("otelgenai: projection limit must be positive")
	}
	if cfg.loggerProviderSet && cfg.loggerProvider == nil {
		return nil, errors.New("otelgenai: logger provider must not be nil")
	}

	loggerProvider := cfg.loggerProvider
	if loggerProvider == nil {
		loggerProvider = global.GetLoggerProvider()
	}
	logger := loggerProvider.Logger(
		cfg.instrumentationName,
		otellog.WithInstrumentationVersion(cfg.instrumentationVersion),
	)

	if cfg.disabled {
		return &Instrumenter{
			tracer:     noop.NewTracerProvider().Tracer(cfg.instrumentationName),
			logger:     logger,
			metrics:    noopMetrics(),
			cfg:        cfg,
			classifier: cfg.errorClassifier,
			diag:       cfg.diagnosticHandler,
			pricing:    cfg.pricingResolver,
		}, nil
	}

	meter := cfg.meterProvider.Meter(
		cfg.instrumentationName,
		metric.WithInstrumentationVersion(cfg.instrumentationVersion),
	)
	mt, err := newMetrics(meter)
	if err != nil {
		return nil, fmt.Errorf("otelgenai: create metrics: %w", err)
	}

	tracer := cfg.tracerProvider.Tracer(
		cfg.instrumentationName,
		trace.WithInstrumentationVersion(cfg.instrumentationVersion),
	)

	return &Instrumenter{
		tracer:     tracer,
		logger:     logger,
		metrics:    mt,
		cfg:        cfg,
		classifier: cfg.errorClassifier,
		diag:       cfg.diagnosticHandler,
		pricing:    cfg.pricingResolver,
	}, nil
}

// hasProjector reports whether a content projector is configured.
func (in *Instrumenter) hasProjector() bool {
	return in.cfg.contentProjector != nil
}
