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
	inner     *ssestream.Stream[anth.MessageStreamEventUnion]
	state     *otelgenai.StreamState
	op        *otelgenai.InferenceOperation
	in        *otelgenai.Instrumenter
	accumResp otelgenai.Response
	hasResp   bool
	terminal  bool
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
		s.finalize(s.terminal, s.inner.Err())
		return false
	}
	event := s.inner.Current()
	// Observe output chunks only for content_block_delta events.
	if event.Type == "content_block_delta" {
		s.state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	}
	// Accumulate model and ID from message_start.
	if event.Type == "message_start" {
		start := event.AsMessageStart()
		s.accumResp.Model = string(start.Message.Model)
		s.accumResp.ID = start.Message.ID
		s.hasResp = true
	}
	// Accumulate cumulative usage and stop reason from message_delta.
	if event.Type == "message_delta" {
		delta := event.AsMessageDelta()
		s.accumResp.Usage = mapDeltaUsage(delta.Usage)
		if string(delta.Delta.StopReason) != "" {
			s.accumResp.FinishReasons = []string{string(delta.Delta.StopReason)}
		}
		s.hasResp = true
		s.terminal = true
	}
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
	s.finalize(s.terminal, err)
	return err
}

// finalize finalizes the stream telemetry. It is idempotent and safe
// to call from multiple terminal paths.
func (s *MessageStream) finalize(terminal bool, err error) {
	var resp otelgenai.Response
	if s.hasResp {
		resp = s.accumResp
	}
	s.state.FinalizeStream(terminal, resp, err)
}
