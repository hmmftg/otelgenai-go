package otelgenai_test

import (
	"context"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// BenchmarkInference_Disabled measures the overhead of a disabled
// (no-op) inference operation.
func BenchmarkInference_Disabled(b *testing.B) {
	instr, err := otelgenai.New(otelgenai.Disabled())
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, op := instr.StartInference(ctx, req)
		op.End(otelgenai.Response{Model: "gpt-4o"}, nil)
	}
}

// BenchmarkInference_Recording measures the overhead of a recording
// inference operation with an in-memory exporter.
func BenchmarkInference_Recording(b *testing.B) {
	instr, exporter := newTestInstrumenter(&testing.T{})
	ctx := context.Background()
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, op := instr.StartInference(ctx, req)
		op.End(otelgenai.Response{Model: "gpt-4o"}, nil)
	}
	_ = exporter
}

// BenchmarkChunkObserve measures the overhead of observing a single
// output chunk.
func BenchmarkChunkObserve(b *testing.B) {
	instr, _ := newTestInstrumenter(&testing.T{})
	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)
	chunk := otelgenai.Chunk{IsOutput: true}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		state.ObserveChunk(chunk)
	}
}
