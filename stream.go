package otelgenai

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// Chunk represents a model output chunk observed during streaming.
// Only actual content deltas should be passed to ObserveChunk, not
// heartbeats, metadata-only events, or terminal markers.
type Chunk struct {
	// IsOutput reports whether this chunk contains model output.
	// Only output chunks trigger timing measurements.
	IsOutput bool
}

// StreamState tracks streaming timing and finalization state for an
// InferenceOperation. It is embedded in provider stream wrappers.
type StreamState struct {
	mu             sync.Mutex
	finalized      bool
	op             *InferenceOperation
	in             *Instrumenter
	ctx            context.Context
	operation      Operation
	provider       string
	model          string
	startTime      time.Time
	firstChunkTime time.Time
	hasFirstChunk  bool
	prevChunkTime  time.Time
}

// NewStreamState creates a StreamState for the given inference operation.
func NewStreamState(op *InferenceOperation) *StreamState {
	if op == nil {
		return nil
	}
	return &StreamState{
		op:        op,
		in:        op.in,
		ctx:       op.ctx,
		operation: op.operation,
		provider:  op.provider,
		model:     op.model,
		startTime: op.startTime,
	}
}

// ObserveChunk records timing for an output chunk. It should be called
// only for chunks containing model output, not heartbeats or metadata.
func (s *StreamState) ObserveChunk(c Chunk) {
	if s == nil || !c.IsOutput {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return
	}
	now := time.Now()
	if !s.hasFirstChunk {
		s.firstChunkTime = now
		s.hasFirstChunk = true
		s.op.hasFirstChunk = true
		s.op.firstChunkTime = now
	} else {
		// Record time between output chunks.
		interval := now.Sub(s.prevChunkTime).Seconds()
		s.in.metrics.clientOperationTimePerOutputChunk.Record(
			s.ctx, interval,
			metric.WithAttributes(
				attribute.String(semconv.AttrGenAIOperationName, string(s.operation)),
				attribute.String(semconv.AttrGenAISystem, s.provider),
				attribute.String(semconv.AttrGenAIRequestModel, s.model),
			),
		)
	}
	s.prevChunkTime = now
}

// FinalizeStream finalizes streaming telemetry. It is idempotent and
// safe to call from multiple terminal paths (End, stream error,
// cancellation, Close). The terminal parameter indicates whether the
// stream reached a terminal provider event (true) or was closed early
// (false). The resp parameter carries accumulated response metadata
// (usage, model, ID, finish reasons) extracted from stream events. The
// err parameter is the terminal error if any.
func (s *StreamState) FinalizeStream(terminal bool, resp Response, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	s.finalized = true
	s.mu.Unlock()

	// Record time-to-first-chunk metric.
	if s.hasFirstChunk {
		ttfb := s.firstChunkTime.Sub(s.startTime).Seconds()
		s.in.metrics.clientOperationTimeToFirstChunk.Record(
			s.ctx, ttfb,
			metric.WithAttributes(
				attribute.String(semconv.AttrGenAIOperationName, string(s.operation)),
				attribute.String(semconv.AttrGenAISystem, s.provider),
				attribute.String(semconv.AttrGenAIRequestModel, s.model),
			),
		)
	}

	finalErr := err
	if !terminal && err == nil {
		// Stream closed before terminal event with no error: treat as
		// cancelled.
		finalErr = errStreamClosedEarly
	}

	// If there was an error and no terminal event, set finish reason.
	if !terminal {
		resp.FinishReasons = []string{"error"}
	}

	s.op.End(resp, finalErr)
}

// errStreamClosedEarly is a sentinel used to classify early stream
// closure as cancelled.
var errStreamClosedEarly = streamClosedError{}

type streamClosedError struct{}

func (streamClosedError) Error() string { return "stream closed before terminal event" }
