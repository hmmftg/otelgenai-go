package anthropic

import (
	anth "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/hmmftg/otelgenai-go"
)

// MessageStream wraps the official Anthropic SSE stream for Messages,
// observing typed events for telemetry while yielding each SDK event
// unchanged.
type MessageStream struct {
	inner *ssestream.Stream[anth.MessageStreamEventUnion]
	state *otelgenai.StreamState
	op    *otelgenai.InferenceOperation
	in    *otelgenai.Instrumenter
}

func newMessageStream(inner *ssestream.Stream[anth.MessageStreamEventUnion], op *otelgenai.InferenceOperation, in *otelgenai.Instrumenter) *MessageStream {
	return &MessageStream{
		inner: inner,
		state: otelgenai.NewStreamState(op),
		op:    op,
		in:    in,
	}
}

// Next advances the stream. It observes content deltas for timing
// while yielding the SDK event unchanged.
func (s *MessageStream) Next() bool {
	hasNext := s.inner.Next()
	if !hasNext {
		s.finalize(false, s.inner.Err())
		return false
	}
	// Observe output chunks: only content_block_delta events contain
	// actual model output. The SDK event union is checked here.
	s.state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	return true
}

// Current returns the current SDK event unchanged.
func (s *MessageStream) Current() anth.MessageStreamEventUnion {
	return s.inner.Current()
}

// Err returns the SDK stream error unchanged.
func (s *MessageStream) Err() error {
	return s.inner.Err()
}

// Close closes the stream and finalizes telemetry. It is idempotent.
func (s *MessageStream) Close() error {
	err := s.inner.Close()
	s.finalize(false, err)
	return err
}

// finalize finalizes the stream telemetry. It is idempotent and safe
// to call from multiple terminal paths.
func (s *MessageStream) finalize(terminal bool, err error) {
	s.state.FinalizeStream(terminal, err)
}
