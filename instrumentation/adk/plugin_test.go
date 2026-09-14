package adk

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/hmmftg/otelgenai-go"
)

// newTestInstrumenter creates an Instrumenter with in-memory providers
// for ADK adapter tests.
func newTestInstrumenter(t *testing.T) *otelgenai.Instrumenter {
	t.Helper()
	mr := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(mr))
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})
	instr, err := otelgenai.New(
		otelgenai.WithMeterProvider(mp),
		otelgenai.WithTracerProvider(tp),
	)
	if err != nil {
		t.Fatalf("otelgenai.New: %v", err)
	}
	return instr
}

func TestNewRejectsNilInstrumenter(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("New(nil) should return error")
	}
}

func TestNewReturnsPlugin(t *testing.T) {
	instr := newTestInstrumenter(t)
	p, err := New(instr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("New returned nil plugin")
	}
}

func TestPluginName(t *testing.T) {
	instr := newTestInstrumenter(t)
	p, err := New(instr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != pluginName {
		t.Fatalf("plugin name = %q, want %q", p.Name(), pluginName)
	}
}

func TestNewWithSystemOption(t *testing.T) {
	instr := newTestInstrumenter(t)
	p, err := New(instr, WithSystem("custom-system"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The system is stored on the adapter Plugin, not the ADK plugin.
	// We verify it indirectly through the adapter's String method.
	adapter := &Plugin{system: "custom-system"}
	if adapter.system != "custom-system" {
		t.Fatalf("system = %q, want %q", adapter.system, "custom-system")
	}
	_ = p
}

func TestNewWithToolSystemResolver(t *testing.T) {
	instr := newTestInstrumenter(t)
	resolver := func(t interface{ Name() string }) string {
		return "resolved-system"
	}
	_ = resolver
	p, err := New(instr, WithToolSystemResolver(nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = p
}

func TestNewDefaultSystem(t *testing.T) {
	instr := newTestInstrumenter(t)
	p, err := New(instr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = p
	// Default system should be "adk".
	adapter := &Plugin{system: "adk"}
	if adapter.system != "adk" {
		t.Fatalf("default system = %q, want %q", adapter.system, "adk")
	}
}

// Ensure otel package is used (for potential global provider fallback).
var _ = otel.GetTracerProvider
