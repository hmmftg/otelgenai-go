package openai

import (
	"context"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/responses"

	"github.com/hmmftg/otelgenai-go"
)

// Responses wraps the official OpenAI Responses service with GenAI
// instrumentation. It creates one logical span per operation that
// encloses all automatic SDK retries.
type Responses struct {
	client       *oai.Client
	instrumenter *otelgenai.Instrumenter
}

// NewResponses creates a typed service wrapper around the official
// OpenAI Responses service.
func NewResponses(client *oai.Client, instr *otelgenai.Instrumenter) *Responses {
	return &Responses{client: client, instrumenter: instr}
}

// New wraps ResponseService.New with instrumentation. It forwards all
// request options unchanged and returns the official response type
// unchanged.
func (s *Responses) New(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) (*responses.Response, error) {
	req := mapResponseRequest(params, false)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	resp, err := s.client.Responses.New(ctx, params, opts...)
	op.End(mapResponseResponse(resp), err)
	return resp, err
}

// NewStreaming wraps ResponseService.NewStreaming with instrumentation.
// It returns a wrapped stream that observes typed events while yielding
// each SDK event unchanged.
func (s *Responses) NewStreaming(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) *ResponseStream {
	req := mapResponseRequest(params, true)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	inner := s.client.Responses.NewStreaming(ctx, params, opts...)
	return newResponseStream(inner, op, s.instrumenter)
}

// ResponseStream wraps the official SSE stream for Responses, observing
// typed events for telemetry while yielding each SDK event unchanged.
type ResponseStream struct {
	inner     *ssestream.Stream[responses.ResponseStreamEventUnion]
	state     *otelgenai.StreamState
	op        *otelgenai.InferenceOperation
	in        *otelgenai.Instrumenter
	accumResp otelgenai.Response
	hasResp   bool
	terminal  bool
}

func newResponseStream(inner *ssestream.Stream[responses.ResponseStreamEventUnion], op *otelgenai.InferenceOperation, in *otelgenai.Instrumenter) *ResponseStream {
	return &ResponseStream{
		inner: inner,
		state: otelgenai.NewStreamState(op),
		op:    op,
		in:    in,
	}
}

// Next advances the stream. It observes content deltas for timing
// while yielding the SDK event unchanged.
func (s *ResponseStream) Next() bool {
	hasNext := s.inner.Next()
	if !hasNext {
		s.finalize(s.terminal, s.inner.Err())
		return false
	}
	event := s.inner.Current()
	// Observe output chunks only for text delta events.
	if event.Type == "response.output_text.delta" {
		s.state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	}
	// Accumulate usage and metadata from the completed event.
	if event.Type == "response.completed" {
		completed := event.AsResponseCompleted()
		s.accumResp = mapResponseResponse(&completed.Response)
		s.hasResp = true
		s.terminal = true
	}
	return true
}

// Current returns the current SDK event unchanged.
func (s *ResponseStream) Current() responses.ResponseStreamEventUnion {
	return s.inner.Current()
}

// Err returns the SDK stream error unchanged.
func (s *ResponseStream) Err() error {
	return s.inner.Err()
}

// Close closes the stream and finalizes telemetry. It is idempotent.
func (s *ResponseStream) Close() error {
	err := s.inner.Close()
	s.finalize(s.terminal, err)
	return err
}

// finalize finalizes the stream telemetry. It is idempotent and safe
// to call from multiple terminal paths.
func (s *ResponseStream) finalize(terminal bool, err error) {
	var resp otelgenai.Response
	if s.hasResp {
		resp = s.accumResp
	}
	s.state.FinalizeStream(terminal, resp, err)
}
