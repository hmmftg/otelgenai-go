package testutil

import (
	"context"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Recorder wires an in-memory span exporter and a manual metric reader
// into OTel SDK providers that can be passed to otelgenai.New via
// WithTracerProvider and WithMeterProvider.
//
// Spans are collected in an in-memory exporter. Metrics are collected
// via a ManualReader; call Collect to retrieve the current metric data.
//
// The recorder does not support resetting cumulative metrics while an
// existing instrumenter retains the old provider. Tests needing
// isolation should create a fresh recorder.
type Recorder struct {
	spanExporter   *tracetest.InMemoryExporter
	tracerProvider *sdktrace.TracerProvider
	metricReader   *sdkmetric.ManualReader
	meterProvider  *sdkmetric.MeterProvider
}

// NewRecorder creates a Recorder with an in-memory span exporter and
// a manual metric reader, wired into fresh SDK providers.
func NewRecorder() *Recorder {
	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(spanExporter)),
	)
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricReader),
	)
	return &Recorder{
		spanExporter:   spanExporter,
		tracerProvider: tracerProvider,
		metricReader:   metricReader,
		meterProvider:  meterProvider,
	}
}

// TracerProvider returns the SDK tracer provider backed by the
// in-memory span exporter.
func (r *Recorder) TracerProvider() oteltrace.TracerProvider {
	return r.tracerProvider
}

// MeterProvider returns the SDK meter provider backed by the manual
// metric reader.
func (r *Recorder) MeterProvider() metric.MeterProvider {
	return r.meterProvider
}

// Spans returns immutable snapshots of all recorded spans in the order
// they were ended.
func (r *Recorder) Spans() []sdktrace.ReadOnlySpan {
	return r.spanExporter.GetSpans().Snapshots()
}

// Span returns the recorded span at the given index, or nil when out
// of range.
func (r *Recorder) Span(index int) sdktrace.ReadOnlySpan {
	spans := r.Spans()
	if index < 0 || index >= len(spans) {
		return nil
	}
	return spans[index]
}

// Collect collects current metric data from the manual reader.
func (r *Recorder) Collect(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var rm metricdata.ResourceMetrics
	err := r.metricReader.Collect(ctx, &rm)
	return rm, err
}

// Shutdown releases both providers and the metric reader, joining
// failures safely.
func (r *Recorder) Shutdown(ctx context.Context) error {
	var errs []error
	if err := r.tracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := r.meterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	for _, e := range errs {
		return e
	}
	return nil
}
