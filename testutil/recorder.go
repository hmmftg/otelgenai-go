package testutil

import (
	"context"
	"sync"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
	logExporter    *inMemoryLogExporter
	loggerProvider *sdklog.LoggerProvider
}

// inMemoryLogExporter stores exported log records in memory.
type inMemoryLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *inMemoryLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (e *inMemoryLogExporter) Shutdown(context.Context) error   { return nil }
func (e *inMemoryLogExporter) ForceFlush(context.Context) error { return nil }

func (e *inMemoryLogExporter) Records() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]sdklog.Record, len(e.records))
	copy(out, e.records)
	return out
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
	logExporter := &inMemoryLogExporter{}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExporter)),
	)
	return &Recorder{
		spanExporter:   spanExporter,
		tracerProvider: tracerProvider,
		metricReader:   metricReader,
		meterProvider:  meterProvider,
		logExporter:    logExporter,
		loggerProvider: loggerProvider,
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

// LoggerProvider returns the SDK logger provider backed by the
// in-memory log exporter.
func (r *Recorder) LoggerProvider() otellog.LoggerProvider {
	return r.loggerProvider
}

// Records returns snapshots of all recorded log records in export
// order.
func (r *Recorder) Records() []sdklog.Record {
	return r.logExporter.Records()
}

// RecordsWithEventName returns recorded log records carrying the given
// event name.
func (r *Recorder) RecordsWithEventName(name string) []sdklog.Record {
	var out []sdklog.Record
	for _, rec := range r.Records() {
		if rec.EventName() == name {
			out = append(out, rec)
		}
	}
	return out
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
	if err := r.loggerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	for _, e := range errs {
		return e
	}
	return nil
}
