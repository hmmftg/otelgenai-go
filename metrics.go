package otelgenai

import (
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// metrics holds all metric instruments created at Instrumenter
// construction. Creating all instruments up front ensures validation
// failures surface immediately rather than at first use.
type metrics struct {
	clientOperationDuration           metric.Float64Histogram
	clientTokenUsage                  metric.Int64Histogram
	clientOperationTimeToFirstChunk   metric.Float64Histogram
	clientOperationTimePerOutputChunk metric.Float64Histogram
	invokeAgentDuration               metric.Float64Histogram
	invokeAgentInferenceCalls         metric.Int64Histogram
	invokeAgentToolCalls              metric.Int64Histogram
	executeToolDuration               metric.Float64Histogram
}

// noopMetrics returns a metrics instance backed by a noop meter. All
// recording is free and produces no observable output.
func noopMetrics() metrics {
	m := noop.NewMeterProvider().Meter("noop")
	mt, _ := newMetricsFromMeter(m)
	return mt
}

// newMetrics creates all metric instruments from the given meter.
func newMetrics(m metric.Meter) (metrics, error) {
	return newMetricsFromMeter(m)
}

func newMetricsFromMeter(m metric.Meter) (metrics, error) {
	var mt metrics
	var err error

	mt.clientOperationDuration, err = m.Float64Histogram(
		semconv.MetricClientOperationDuration,
		metric.WithUnit(semconv.UnitSeconds),
		metric.WithDescription(semconv.DescClientOperationDuration),
		metric.WithExplicitBucketBoundaries(semconv.DurationBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.clientTokenUsage, err = m.Int64Histogram(
		semconv.MetricClientTokenUsage,
		metric.WithUnit(semconv.UnitTokens),
		metric.WithDescription(semconv.DescClientTokenUsage),
		metric.WithExplicitBucketBoundaries(semconv.TokenUsageBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.clientOperationTimeToFirstChunk, err = m.Float64Histogram(
		semconv.MetricClientOperationTimeToFirstChunk,
		metric.WithUnit(semconv.UnitSeconds),
		metric.WithDescription(semconv.DescClientOperationTimeToFirstChunk),
		metric.WithExplicitBucketBoundaries(semconv.TimeToFirstChunkBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.clientOperationTimePerOutputChunk, err = m.Float64Histogram(
		semconv.MetricClientOperationTimePerOutputChunk,
		metric.WithUnit(semconv.UnitSeconds),
		metric.WithDescription(semconv.DescClientOperationTimePerOutputChunk),
		metric.WithExplicitBucketBoundaries(semconv.TimePerOutputChunkBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.invokeAgentDuration, err = m.Float64Histogram(
		semconv.MetricInvokeAgentDuration,
		metric.WithUnit(semconv.UnitSeconds),
		metric.WithDescription(semconv.DescInvokeAgentDuration),
		metric.WithExplicitBucketBoundaries(semconv.DurationBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.invokeAgentInferenceCalls, err = m.Int64Histogram(
		semconv.MetricInvokeAgentInferenceCalls,
		metric.WithUnit(semconv.UnitInferenceCalls),
		metric.WithDescription(semconv.DescInvokeAgentInferenceCalls),
		metric.WithExplicitBucketBoundaries(semconv.CountBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.invokeAgentToolCalls, err = m.Int64Histogram(
		semconv.MetricInvokeAgentToolCalls,
		metric.WithUnit(semconv.UnitToolCalls),
		metric.WithDescription(semconv.DescInvokeAgentToolCalls),
		metric.WithExplicitBucketBoundaries(semconv.CountBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	mt.executeToolDuration, err = m.Float64Histogram(
		semconv.MetricExecuteToolDuration,
		metric.WithUnit(semconv.UnitSeconds),
		metric.WithDescription(semconv.DescExecuteToolDuration),
		metric.WithExplicitBucketBoundaries(semconv.DurationBoundaries...),
	)
	if err != nil {
		return mt, err
	}
	return mt, nil
}
