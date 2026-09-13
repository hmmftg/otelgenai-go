package otelgenai_test

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// TestStreaming_NilSafe verifies that all StreamState methods are
// nil-safe: calling methods on a nil *StreamState must not panic.
func TestStreaming_NilSafe(t *testing.T) {
	var state *otelgenai.StreamState

	// ObserveChunk on nil should not panic.
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})

	// FinalizeStream on nil should not panic.
	state.FinalizeStream(true, otelgenai.Response{}, nil)
	state.FinalizeStream(false, otelgenai.Response{}, nil)
}

// TestStreaming_NewStreamStateNilOperation verifies that NewStreamState
// returns nil for a nil operation (disabled instrumentation).
func TestStreaming_NewStreamStateNilOperation(t *testing.T) {
	state := otelgenai.NewStreamState(nil)
	if state != nil {
		t.Error("expected nil StreamState for nil operation")
	}
}

// TestStreaming_ObserveChunkAfterFinalize verifies that chunks observed
// after finalization are silently ignored (no panic, no metric).
func TestStreaming_ObserveChunkAfterFinalize(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// Finalize first.
	state.FinalizeStream(true, otelgenai.Response{}, nil)

	// Observe chunks after finalization - should be ignored.
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

// TestStreaming_DisabledInstrumenter verifies that streaming with a
// disabled instrumenter produces no spans and no metrics.
func TestStreaming_DisabledInstrumenter(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)
	// Replace with a disabled instrumenter using the same exporter.
	disabledInstr, err := otelgenai.New(
		otelgenai.Disabled(),
		otelgenai.WithTracerProvider(newTestTracerProvider(exporter)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	instr = disabledInstr

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// All operations should be nil-safe no-ops.
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	state.FinalizeStream(true, otelgenai.Response{}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans when disabled, got %d", len(spans))
	}
}
