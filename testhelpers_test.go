package otelgenai_test

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestTracerProvider creates a tracer provider that exports to the
// given in-memory exporter.
func newTestTracerProvider(exporter *tracetest.InMemoryExporter) *trace.TracerProvider {
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)),
	)
	return tp
}

func strVal(s string) attribute.Value {
	return attribute.StringValue(s)
}

func int64Val(i int64) attribute.Value {
	return attribute.Int64Value(i)
}

func float64Val(f float64) attribute.Value {
	return attribute.Float64Value(f)
}
