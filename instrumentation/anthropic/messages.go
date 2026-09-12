// Package anthropic provides typed Anthropic service wrappers that
// instrument the official github.com/anthropics/anthropic-sdk-go SDK
// with OpenTelemetry GenAI traces and metrics.
//
// The wrappers create one logical span per provider operation that
// encloses all automatic SDK retries. They forward official request
// options unchanged and return official response types unchanged.
// They do not parse wire bodies, read authorization headers, or alter
// retry settings.
package anthropic

import (
	"context"

	anth "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/hmmftg/otelgenai-go"
)

// Messages wraps the official Anthropic MessageService with GenAI
// instrumentation. It creates one logical span per operation that
// encloses all automatic SDK retries.
type Messages struct {
	client       *anth.Client
	instrumenter *otelgenai.Instrumenter
}

// NewMessages creates a typed service wrapper around the official
// Anthropic Messages service.
func NewMessages(client *anth.Client, instr *otelgenai.Instrumenter) *Messages {
	return &Messages{client: client, instrumenter: instr}
}

// New wraps MessageService.New with instrumentation. It forwards all
// request options unchanged and returns the official response type
// unchanged.
func (s *Messages) New(ctx context.Context, params anth.MessageNewParams, opts ...option.RequestOption) (*anth.Message, error) {
	req := mapMessageRequest(params, false)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	resp, err := s.client.Messages.New(ctx, params, opts...)
	op.End(mapMessageResponse(resp), err)
	return resp, err
}

// NewStreaming wraps MessageService.NewStreaming with instrumentation.
// It returns a wrapped stream that observes typed events while
// yielding each SDK event unchanged.
func (s *Messages) NewStreaming(ctx context.Context, params anth.MessageNewParams, opts ...option.RequestOption) *MessageStream {
	req := mapMessageRequest(params, true)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	inner := s.client.Messages.NewStreaming(ctx, params, opts...)
	return newMessageStream(inner, op, s.instrumenter)
}
