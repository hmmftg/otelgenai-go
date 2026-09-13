package openai

import (
	"context"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/hmmftg/otelgenai-go"
)

// Embeddings wraps the official OpenAI EmbeddingService with GenAI
// instrumentation. It creates one logical span per operation that
// encloses all automatic SDK retries.
type Embeddings struct {
	client       *oai.Client
	instrumenter *otelgenai.Instrumenter
	system       string
}

// NewEmbeddings creates a typed service wrapper around the official
// OpenAI Embeddings service. Optional adapter options (e.g. WithSystem)
// may be passed to override the default gen_ai.system value for
// OpenAI-compatible providers.
func NewEmbeddings(client *oai.Client, instr *otelgenai.Instrumenter, opts ...Option) *Embeddings {
	return &Embeddings{
		client:       client,
		instrumenter: instr,
		system:       resolveSystem(opts),
	}
}

// New wraps EmbeddingService.New with instrumentation. It forwards all
// request options unchanged and returns the official response type
// unchanged.
func (s *Embeddings) New(ctx context.Context, params oai.EmbeddingNewParams, opts ...option.RequestOption) (*oai.CreateEmbeddingResponse, error) {
	req := mapEmbeddingRequest(params, s.system)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	resp, err := s.client.Embeddings.New(ctx, params, opts...)
	op.End(mapEmbeddingResponse(resp), err)
	return resp, err
}
