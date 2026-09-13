// Package googlegenai provides typed Google GenAI service wrappers that
// instrument the official google.golang.org/genai SDK with OpenTelemetry
// GenAI traces and metrics.
//
// The wrappers create one logical span per provider operation that
// encloses all automatic SDK retries. They forward official request
// options unchanged and return official response types unchanged.
// They do not parse wire bodies, read authorization headers, or alter
// retry settings.
//
// Backend-aware attribution: the gen_ai.system attribute is resolved
// from the client's configured backend (gcp.gemini for Gemini API,
// gcp.vertex_ai for Vertex AI, gcp.gen_ai as a safe fallback).
package googlegenai

import (
	"context"
	"iter"

	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
)

// Models wraps the official Google GenAI Models service with GenAI
// instrumentation. It creates one logical span per operation that
// encloses all automatic SDK retries.
type Models struct {
	client       *genai.Client
	instrumenter *otelgenai.Instrumenter
	system       string
}

// NewModels creates a typed service wrapper around the official Google
// GenAI Models service. The gen_ai.system attribute is resolved from
// the client's configured backend.
func NewModels(client *genai.Client, instr *otelgenai.Instrumenter) *Models {
	return &Models{
		client:       client,
		instrumenter: instr,
		system:       ResolveSystem(client.ClientConfig().Backend),
	}
}

// GenerateContent wraps Models.GenerateContent with instrumentation. It
// forwards all parameters unchanged and returns the official response
// type unchanged.
func (s *Models) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	req := mapGenerateContentRequest(model, config, false, s.system)
	ctx, op := s.instrumenter.StartInference(ctx, req)
	resp, err := s.client.Models.GenerateContent(ctx, model, contents, config)
	op.End(mapGenerateContentResponse(resp), err)
	return resp, err
}

// GenerateContentStream wraps Models.GenerateContentStream with
// instrumentation. It returns an instrumented iter.Seq2 that yields
// each SDK response/error pair unchanged.
//
// The inference operation starts when iteration begins (not when the
// sequence is constructed), so an unconsumed sequence creates no span.
// The SDK stream is constructed after StartInference so the SDK request
// executes with the span-enriched context.
//
// The returned sequence is single-use, matching the underlying SDK
// iterator semantics. If it is never ranged, no iterator callback runs
// and no operation is started.
func (s *Models) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	req := mapGenerateContentRequest(model, config, true, s.system)
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		ctx, op := s.instrumenter.StartInference(ctx, req)
		state := otelgenai.NewStreamState(op)
		var accumResp otelgenai.Response
		var terminal bool
		var streamErr error
		defer func() { state.FinalizeStream(terminal, accumResp, streamErr) }()

		// Construct the SDK stream AFTER StartInference so it captures
		// the span-enriched context, not the original caller context.
		inner := s.client.Models.GenerateContentStream(ctx, model, contents, config)

		for resp, err := range inner {
			if err != nil {
				streamErr = err
				terminal = true
				state.FinalizeStream(terminal, accumResp, streamErr)
				yield(resp, err) // forward exact (resp, err) pair
				return
			}
			// Observe output chunks only for responses containing
			// generated content, not metadata-only responses.
			if HasGeneratedOutput(resp) {
				state.ObserveChunk(otelgenai.Chunk{IsOutput: true})
			}
			// Accumulate latest response metadata.
			accumResp = mapGenerateContentResponse(resp)
			// Check for terminal finish reason.
			if resp != nil {
				for _, c := range resp.Candidates {
					if c != nil && string(c.FinishReason) != "" {
						terminal = true
						break
					}
				}
			}
			if !yield(resp, nil) {
				return
			}
		}
		terminal = true
	}
}
