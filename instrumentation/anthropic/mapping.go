package anthropic

import (
	anth "github.com/anthropics/anthropic-sdk-go"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// mapMessageResponse converts an Anthropic Message to the
// provider-neutral otelgenai.Response.
func mapMessageResponse(r *anth.Message) otelgenai.Response {
	resp := otelgenai.Response{
		Model: string(r.Model),
		ID:    r.ID,
	}
	if r.StopReason != "" {
		resp.FinishReasons = []string{string(r.StopReason)}
	}
	resp.Usage = mapUsage(r.Usage)
	return resp
}

// mapUsage converts an Anthropic Usage to the provider-neutral
// otelgenai.Usage. Cache creation and cache read tokens are mapped
// carefully to avoid double counting: cache read tokens are a subset
// of input tokens, and cache creation tokens are also a subset of
// input tokens. We report them as separate convention attributes
// without adding them to the aggregate input count.
func mapUsage(u anth.Usage) otelgenai.Usage {
	return otelgenai.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
	}
}

// mapMessageRequest builds the provider-neutral otelgenai.Request from
// Message parameters.
func mapMessageRequest(params anth.MessageNewParams, streaming bool) otelgenai.Request {
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationChat),
		Provider:  semconv.GenAISystemAnthropic,
		Model:     string(params.Model),
		Streaming: streaming,
	}
	if params.MaxTokens > 0 {
		req.MaxTokens = params.MaxTokens
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
