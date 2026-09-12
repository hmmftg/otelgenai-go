package otelgenai_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

func TestStreaming_FirstChunkTiming(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// Observe a few output chunks.
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	state.ObserveChunk(otelgenai.Chunk{IsOutput: true})

	// Finalize with terminal success.
	state.FinalizeStream(true, otelgenai.Response{}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Time to first chunk should be recorded.
	found := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIResponseTimeToFirstChunk {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gen_ai.response.time_to_first_chunk attribute")
	}
}

func TestStreaming_EarlyClose(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// Close before terminal event.
	state.FinalizeStream(false, otelgenai.Response{}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// Should have error status and finish_reason "error".
	foundError := false
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrErrorType {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected error.type attribute for early close")
	}
}

func TestStreaming_IdempotentFinalize(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// Finalize multiple times.
	state.FinalizeStream(true, otelgenai.Response{}, nil)
	state.FinalizeStream(true, otelgenai.Response{}, nil)
	state.FinalizeStream(false, otelgenai.Response{}, errors.New("late error"))

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestStreaming_ConcurrentFinalize(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state.FinalizeStream(true, otelgenai.Response{}, nil)
		}()
	}
	wg.Wait()

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestStreaming_NonOutputChunksNotTimed(t *testing.T) {
	instr, exporter := newTestInstrumenter(t)

	ctx := context.Background()
	_, op := instr.StartInference(ctx, otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  "openai",
		Model:     "gpt-4o",
		Streaming: true,
	})
	state := otelgenai.NewStreamState(op)

	// Non-output chunks should not trigger timing.
	state.ObserveChunk(otelgenai.Chunk{IsOutput: false})
	state.ObserveChunk(otelgenai.Chunk{IsOutput: false})

	state.FinalizeStream(true, otelgenai.Response{}, nil)

	spans := exporter.GetSpans().Snapshots()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	// No first-chunk time should be recorded.
	for _, attr := range span.Attributes() {
		if string(attr.Key) == semconv.AttrGenAIResponseTimeToFirstChunk {
			t.Error("expected no time_to_first_chunk for non-output chunks")
		}
	}
}
