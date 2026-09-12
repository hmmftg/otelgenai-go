// Package conformancetest provides internal test helpers for asserting
// exact semantic-convention compliance of spans and metrics. These
// helpers are internal in v0.1 and may become public in a later
// release once the core API stabilizes.
package conformancetest

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// AssertSpanName checks that a span has the expected name.
func AssertSpanName(t *testing.T, span sdktrace.ReadOnlySpan, expected string) {
	t.Helper()
	if span.Name() != expected {
		t.Errorf("span name: got %q, want %q", span.Name(), expected)
	}
}

// AssertSpanKind checks that a span has the expected kind.
func AssertSpanKind(t *testing.T, span sdktrace.ReadOnlySpan, expected trace.SpanKind) {
	t.Helper()
	if span.SpanKind() != expected {
		t.Errorf("span kind: got %v, want %v", span.SpanKind(), expected)
	}
}

// AssertSpanStatus checks that a span has the expected status code.
func AssertSpanStatus(t *testing.T, span sdktrace.ReadOnlySpan, expected codes.Code) {
	t.Helper()
	if span.Status().Code != expected {
		t.Errorf("span status: got %v, want %v", span.Status().Code, expected)
	}
}

// AssertAttr checks that a span has an attribute with the expected key
// and value.
func AssertAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string, expected attribute.Value) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			if attr.Value.Type() == expected.Type() && attr.Value.Emit() == expected.Emit() {
				return
			}
			t.Errorf("span attr %q: got %v, want %v", key, attr.Value, expected)
			return
		}
	}
	t.Errorf("span attr %q: not found", key)
}

// AssertAttrNotPresent checks that a span does NOT have an attribute
// with the given key.
func AssertAttrNotPresent(t *testing.T, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			t.Errorf("span attr %q: should not be present but was found with value %v", key, attr.Value)
			return
		}
	}
}

// AssertNoSentinel checks that no span attribute contains any of the
// sentinel secret strings.
func AssertNoSentinel(t *testing.T, span sdktrace.ReadOnlySpan, sentinels []string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		s := attr.Value.Emit()
		for _, sentinel := range sentinels {
			if s == sentinel {
				t.Errorf("span attr %q contains sentinel %q", attr.Key, sentinel)
			}
		}
	}
}

// FindSpan returns the first span with the given name, or nil.
func FindSpan(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	return nil
}
