package mcp

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go/testutil"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestPropagation_RoundTrip(t *testing.T) {
	// Install W3C TraceContext + Baggage propagator.
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	tracer := rec.TracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	// Inject trace context into a new meta.
	meta := InjectTraceContext(ctx, mcp.Meta{"existing": "value"})

	// Verify existing value preserved.
	if v, ok := meta["existing"]; !ok || v != "value" {
		t.Errorf("existing _meta value not preserved: got %v", v)
	}

	// Verify traceparent injected.
	tpVal, ok := meta["traceparent"]
	if !ok {
		t.Fatal("traceparent not found in _meta after injection")
	}
	if tpVal == "" {
		t.Error("traceparent is empty")
	}

	// Extract trace context from meta.
	derivedCtx := ExtractTraceContext(context.Background(), meta)
	spanCtx := oteltrace.SpanContextFromContext(derivedCtx)

	// Verify the extracted span context matches the original span.
	if spanCtx.SpanID() != span.SpanContext().SpanID() {
		t.Errorf("extracted SpanID mismatch: got %q, want %q",
			spanCtx.SpanID(), span.SpanContext().SpanID())
	}
	if spanCtx.TraceID() != span.SpanContext().TraceID() {
		t.Errorf("extracted TraceID mismatch: got %q, want %q",
			spanCtx.TraceID(), span.SpanContext().TraceID())
	}
}

func TestPropagation_OriginalMapNotMutated(t *testing.T) {
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	tracer := rec.TracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	original := mcp.Meta{"existing": "value"}
	_ = InjectTraceContext(ctx, original)

	// Verify the original map was not mutated.
	if v, ok := original["traceparent"]; ok {
		t.Errorf("original map was mutated: traceparent=%v", v)
	}
	if len(original) != 1 {
		t.Errorf("original map size changed: got %d, want 1", len(original))
	}
}

func TestPropagation_NilMeta(t *testing.T) {
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	rec := testutil.NewRecorder()
	defer rec.Shutdown(context.Background())

	tracer := rec.TracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	// Inject into nil meta should create a new map.
	meta := InjectTraceContext(ctx, nil)
	if meta == nil {
		t.Fatal("expected non-nil meta from nil input")
	}
	if _, ok := meta["traceparent"]; !ok {
		t.Error("traceparent not found in _meta from nil input")
	}
}

func TestPropagation_EmptyMetaExtract(t *testing.T) {
	ctx := context.Background()
	derived := ExtractTraceContext(ctx, nil)
	if derived != ctx {
		t.Error("ExtractTraceContext with nil meta should return original ctx")
	}

	derived = ExtractTraceContext(ctx, mcp.Meta{})
	if derived != ctx {
		t.Error("ExtractTraceContext with empty meta should return original ctx")
	}
}

func TestPropagation_NoOpPropagatorSafety(t *testing.T) {
	// With the default/no-op propagator, InjectTraceContext should
	// still return a cloned map and never mutate the input.
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

	original := mcp.Meta{"existing": "value"}
	cloned := InjectTraceContext(context.Background(), original)

	// Cloned map should exist.
	if cloned == nil {
		t.Fatal("expected non-nil cloned map even with no-op propagator")
	}

	// Existing value should be preserved.
	if v, ok := cloned["existing"]; !ok || v != "value" {
		t.Errorf("existing value not preserved in clone: got %v", v)
	}

	// Original should not be mutated.
	if _, ok := original["traceparent"]; ok {
		t.Error("original map was mutated even with no-op propagator")
	}

	// No traceparent should be injected with no-op propagator.
	if _, ok := cloned["traceparent"]; ok {
		t.Error("traceparent injected with no-op propagator")
	}
}
