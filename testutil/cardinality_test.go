package testutil_test

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hmmftg/otelgenai-go/testutil"
)

func TestAssertLowCardinalityModel_CleanModel(t *testing.T) {
	span := makeSpan("chat gpt-4o", []attribute.KeyValue{
		attribute.String("gen_ai.request.model", "gpt-4o"),
	})
	testutil.AssertLowCardinalityModel(t, span) // should not fail
}

func TestAssertLowCardinalityModel_UUIDInModel(t *testing.T) {
	span := makeSpan("chat model", []attribute.KeyValue{
		attribute.String("gen_ai.request.model", "550e8400-e29b-41d4-a716-446655440000"),
	})
	expectFail(t, func(sub *testing.T) {
		testutil.AssertLowCardinalityModel(sub, span)
	})
}

func TestAssertLowCardinalityModel_LongHexInModel(t *testing.T) {
	span := makeSpan("chat model", []attribute.KeyValue{
		attribute.String("gen_ai.request.model", "abcdef0123456789abcdef0123456789"),
	})
	expectFail(t, func(sub *testing.T) {
		testutil.AssertLowCardinalityModel(sub, span)
	})
}

func TestAssertNoDynamicSpanNames_Clean(t *testing.T) {
	spans := toReadOnly([]tracetest.SpanStub{{Name: "chat gpt-4o"}})
	testutil.AssertNoDynamicSpanNames(t, spans)
}

func TestAssertNoDynamicSpanNames_UUIDInName(t *testing.T) {
	spans := toReadOnly([]tracetest.SpanStub{{Name: "chat 550e8400-e29b-41d4-a716-446655440000"}})
	expectFail(t, func(sub *testing.T) {
		testutil.AssertNoDynamicSpanNames(sub, spans)
	})
}

func TestAssertNoResourceURIsInSpanNames_Clean(t *testing.T) {
	spans := toReadOnly([]tracetest.SpanStub{{Name: "resources/read"}})
	testutil.AssertNoResourceURIsInSpanNames(t, spans)
}

func TestAssertNoResourceURIsInSpanNames_URIInName(t *testing.T) {
	spans := toReadOnly([]tracetest.SpanStub{{Name: "file:///path/to/resource"}})
	expectFail(t, func(sub *testing.T) {
		testutil.AssertNoResourceURIsInSpanNames(sub, spans)
	})
}

func TestAssertNoContentInAttributes_Clean(t *testing.T) {
	span := makeSpan("chat gpt-4o", []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", "chat"),
	})
	testutil.AssertNoContentInAttributes(t, span)
}

func TestAssertNoContentInAttributes_WithContent(t *testing.T) {
	span := makeSpan("chat gpt-4o", []attribute.KeyValue{
		attribute.String("gen_ai.input.messages", "secret content"),
	})
	expectFail(t, func(sub *testing.T) {
		testutil.AssertNoContentInAttributes(sub, span)
	})
}

func TestAssertNoRawErrors_Clean(t *testing.T) {
	spans := toReadOnly([]tracetest.SpanStub{{
		Name: "chat gpt-4o",
		Attributes: []attribute.KeyValue{
			attribute.String("error.type", "timeout"),
		},
	}})
	testutil.AssertNoRawErrors(t, spans)
}

// makeSpan creates a ReadOnlySpan from a name and attributes for testing.
func makeSpan(name string, attrs []attribute.KeyValue) sdktrace.ReadOnlySpan {
	return tracetest.SpanStub{
		Name:       name,
		Attributes: attrs,
	}.Snapshot()
}

// toReadOnly converts SpanStubs to ReadOnlySpans.
func toReadOnly(spans []tracetest.SpanStub) []sdktrace.ReadOnlySpan {
	return tracetest.SpanStubs(spans).Snapshots()
}

// expectFail runs a function with a subtest and asserts that the
// subtest fails (reports an error).
func expectFail(t *testing.T, fn func(sub *testing.T)) {
	t.Helper()
	sub := &testing.T{}
	fn(sub)
	if !sub.Failed() {
		t.Errorf("expected assertion to fail, but it passed")
	}
}
