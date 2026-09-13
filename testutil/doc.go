// Package testutil provides test helpers for asserting exact
// semantic-convention compliance of spans and metrics produced by
// otelgenai-go.
//
// The Recorder wires an in-memory span exporter and a manual metric
// reader into OTel SDK providers that can be passed to
// otelgenai.New via WithTracerProvider and WithMeterProvider.
//
// Assertion helpers expose semantic contracts (e.g. AssertTokenUsage)
// and keep OTel SDK metric traversal mechanics private to the
// implementation, so the public testing API is decoupled from the
// exact metricdata representation.
//
// testutil must not become a dependency of production packages:
// production core and adapters must not import testutil; only tests
// import it.
package testutil
