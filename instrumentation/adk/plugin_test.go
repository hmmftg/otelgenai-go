package adk

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/hmmftg/otelgenai-go"
)

// newTestInstrumenter creates an Instrumenter with in-memory providers
// for ADK adapter tests. Accepts additional options for diagnostic
// handlers and other configuration.
func newTestInstrumenter(t *testing.T, opts ...otelgenai.Option) *otelgenai.Instrumenter {
	t.Helper()
	mr := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})
	defaultOpts := []otelgenai.Option{
		otelgenai.WithMeterProvider(mp),
		otelgenai.WithTracerProvider(tp),
	}
	instr, err := otelgenai.New(append(defaultOpts, opts...)...)
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
	if p == nil {
		t.Fatal("New returned nil plugin")
	}
}

func TestNewWithToolSystemResolver(t *testing.T) {
	instr := newTestInstrumenter(t)
	resolver := func(t toolNameProvider) string {
		return "resolved-system"
	}
	p, err := New(instr, WithToolSystemResolver(resolver))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p == nil {
		t.Fatal("New returned nil plugin")
	}
}

func TestNewDefaultSystem(t *testing.T) {
	instr := newTestInstrumenter(t)
	p, err := New(instr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = p
	// Default system should be "adk" (verified through lifecycle tests).
}
