package openai

import (
	"context"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/hmmftg/otelgenai-go"
)

// ChatCompletions wraps the official OpenAI ChatCompletionService with
// GenAI instrumentation. It creates one logical span per operation that
// encloses all automatic SDK retries.
type ChatCompletions struct {
	client      *oai.Client
	instrumenter *otelgenai.Instrumenter
}

// NewChatCompletions creates a typed service wrapper around the
// official OpenAI Chat Completions service.
func NewChatCompletions(client *oai.Client, instr *otelgenai.Instrumenter) *ChatCompletions {
	return &ChatCompletions{client: client, instrumenter: instr}
}

// New wraps ChatCompletionService.New with instrumentation. It forwards
// all request options unchanged and returns the official response type
// unchanged.
func (s *ChatCompletions) New(ctx context.Context, params oai.ChatCompletionNewParams, opts ...option.RequestOption) (*oai.ChatCompletion, error) {
	req := mapChatRequest(params, false)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	resp, err := s.client.Chat.Completions.New(ctx, params, opts...)
	op.End(mapChatCompletionResponse(resp), err)
	return resp, err
}

// NewStreaming wraps ChatCompletionService.NewStreaming with
// instrumentation. It returns a wrapped stream that observes typed
// events while yielding each SDK event unchanged.
func (s *ChatCompletions) NewStreaming(ctx context.Context, params oai.ChatCompletionNewParams, opts ...option.RequestOption) *ChatCompletionStream {
	req := mapChatRequest(params, true)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	inner := s.client.Chat.Completions.NewStreaming(ctx, params, opts...)
	return newChatCompletionStream(inner, op, s.instrumenter)
}

// ChatCompletionStream wraps the official SSE stream for Chat
// Completions, observing typed events for telemetry while yielding
// each SDK event unchanged.
type ChatCompletionStream struct {
	inner *ssestream.Stream[oai.ChatCompletionChunk]
	state *otelgenai.StreamState
	op    *otelgenai.InferenceOperation
	in    *otelgenai.Instrumenter
}

func newChatCompletionStream(inner *ssestream.Stream[oai.ChatCompletionChunk], op *otelgenai.InferenceOperation, in *otelgenai.Instrumenter) *ChatCompletionStream {
	return &ChatCompletionStream{
		inner: inner,
		state: otelgenai.NewStreamState(op),
		op:    op,
		in:    in,
	}
}

// Next advances the stream. It observes content deltas for timing
// while yielding the SDK event unchanged.
func (s *ChatCompletionStream) Next() bool {
	hasNext := s.inner.Next()
	if !hasNext {
		s.finalize(false, s.inner.Err())
		return false
	}
	chunk := s.inner.Current()
	// Observe output chunks: only chunks with content deltas.
	if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
		s.state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
	}
	return true
}

// Current returns the current SDK event unchanged.
func (s *ChatCompletionStream) Current() oai.ChatCompletionChunk {
	return s.inner.Current()
}

// Err returns the SDK stream error unchanged.
func (s *ChatCompletionStream) Err() error {
	return s.inner.Err()
}

// Close closes the stream and finalizes telemetry. It is idempotent.
func (s *ChatCompletionStream) Close() error {
	err := s.inner.Close()
	s.finalize(false, err)
	return err
}

// finalize finalizes the stream telemetry. It is idempotent and safe
// to call from multiple terminal paths.
func (s *ChatCompletionStream) finalize(terminal bool, err error) {
	// Accumulate usage from the last chunk if available.
	var resp otelgenai.Response
	if s.inner.Err() == nil {
		chunk := s.inner.Current()
		if chunk.Usage.TotalTokens > 0 {
			resp.Usage = mapCompletionUsage(chunk.Usage)
		}
		if chunk.ID != "" {
			resp.ID = chunk.ID
		}
		if chunk.Model != "" {
			resp.Model = chunk.Model
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			resp.FinishReasons = []string{chunk.Choices[0].FinishReason}
			terminal = true
		}
	}
	s.state.FinalizeStream(terminal, err)
}
