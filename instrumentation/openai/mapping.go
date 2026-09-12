// Package openai provides typed OpenAI service wrappers that instrument
// the official github.com/openai/openai-go SDK with OpenTelemetry GenAI
// traces and metrics.
//
// The wrappers create one logical span per provider operation that
// encloses all automatic SDK retries. They forward official request
// options unchanged and return official response types unchanged.
// They do not parse wire bodies, read authorization headers, or alter
// retry settings.
//
// Unknown or unsupported OpenAI services remain available through the
// original client and are explicitly uninstrumented.
package openai

import (
	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// mapChatCompletionResponse converts an OpenAI ChatCompletion to the
// provider-neutral otelgenai.Response.
func mapChatCompletionResponse(r *oai.ChatCompletion) otelgenai.Response {
	resp := otelgenai.Response{
		Model: r.Model,
		ID:    r.ID,
	}
	if len(r.Choices) > 0 && r.Choices[0].FinishReason != "" {
		resp.FinishReasons = []string{r.Choices[0].FinishReason}
	}
	resp.Usage = mapCompletionUsage(r.Usage)
	return resp
}

// mapCompletionUsage converts an OpenAI CompletionUsage to the
// provider-neutral otelgenai.Usage.
func mapCompletionUsage(u oai.CompletionUsage) otelgenai.Usage {
	return otelgenai.Usage{
		InputTokens:     u.PromptTokens,
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: u.PromptTokensDetails.CachedTokens,
		ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens,
	}
}

// mapChatRequest builds the provider-neutral otelgenai.Request from
// Chat Completion parameters.
func mapChatRequest(params oai.ChatCompletionNewParams, streaming bool) otelgenai.Request {
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  semconv.GenAISystemOpenAI,
		Model:     string(params.Model),
		Streaming: streaming,
	}
	if params.MaxTokens.Value != 0 {
		req.MaxTokens = params.MaxTokens.Value
	}
	if params.MaxCompletionTokens.Value != 0 {
		req.MaxTokens = params.MaxCompletionTokens.Value
	}
	if params.Temperature.Valid() {
		req.HasTemperature = true
		req.Temperature = params.Temperature.Value
	}
	if params.TopP.Valid() {
		req.HasTopP = true
		req.TopP = params.TopP.Value
	}
	if params.Seed.Valid() {
		req.HasSeed = true
		req.Seed = params.Seed.Value
	}
	return req
}

// mapEmbeddingResponse converts an OpenAI CreateEmbeddingResponse to
// the provider-neutral otelgenai.Response.
func mapEmbeddingResponse(r *oai.CreateEmbeddingResponse) otelgenai.Response {
	return otelgenai.Response{
		Model: r.Model,
		Usage: otelgenai.Usage{
			InputTokens: r.Usage.PromptTokens,
		},
	}
}

// mapEmbeddingRequest builds the provider-neutral otelgenai.Request
// from Embedding parameters.
func mapEmbeddingRequest(params oai.EmbeddingNewParams) otelgenai.Request {
	return otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationEmbeddings),
		Provider:  semconv.GenAISystemOpenAI,
		Model:     string(params.Model),
	}
}

// mapResponseResponse converts an OpenAI Responses API Response to the
// provider-neutral otelgenai.Response.
func mapResponseResponse(r *responses.Response) otelgenai.Response {
	resp := otelgenai.Response{
		Model: string(r.Model),
		ID:    r.ID,
	}
	switch r.Status {
	case responses.ResponseStatusCompleted:
		resp.FinishReasons = []string{"stop"}
	case responses.ResponseStatusFailed:
		resp.FinishReasons = []string{"error"}
	case responses.ResponseStatusCancelled:
		resp.FinishReasons = []string{"cancelled"}
	case responses.ResponseStatusIncomplete:
		resp.FinishReasons = []string{"length"}
	}
	resp.Usage = otelgenai.Usage{
		InputTokens:     r.Usage.InputTokens,
		OutputTokens:    r.Usage.OutputTokens,
		CacheReadTokens: r.Usage.InputTokensDetails.CachedTokens,
		ReasoningTokens: r.Usage.OutputTokensDetails.ReasoningTokens,
	}
	return resp
}

// mapResponseRequest builds the provider-neutral otelgenai.Request from
// Responses API parameters.
func mapResponseRequest(params responses.ResponseNewParams, streaming bool) otelgenai.Request {
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationGenerateContent),
		Provider:  semconv.GenAISystemOpenAI,
		Model:     string(params.Model),
		Streaming: streaming,
	}
	if params.MaxOutputTokens.Valid() {
		req.MaxTokens = params.MaxOutputTokens.Value
	}
	if params.Temperature.Valid() {
		req.HasTemperature = true
		req.Temperature = params.Temperature.Value
	}
	if params.TopP.Valid() {
		req.HasTopP = true
		req.TopP = params.TopP.Value
	}
	return req
}
