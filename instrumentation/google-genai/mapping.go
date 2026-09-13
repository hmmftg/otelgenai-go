package googlegenai

import (
	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// mapGenerateContentRequest builds the provider-neutral otelgenai.Request
// from Google GenAI GenerateContent parameters. Only metadata-only config
// fields are mapped; raw contents, system instruction text, tools, and
// output text are never mapped to preserve safe-by-default behavior.
func mapGenerateContentRequest(model string, config *genai.GenerateContentConfig, streaming bool, system string) otelgenai.Request {
	req := otelgenai.Request{
		Operation: otelgenai.Operation(semconv.OperationGenerateContent),
		Provider:  system,
		Model:     model,
		Streaming: streaming,
	}
	if config == nil {
		return req
	}
	if config.MaxOutputTokens > 0 {
		req.MaxTokens = int64(config.MaxOutputTokens)
	}
	if config.Temperature != nil {
		req.HasTemperature = true
		req.Temperature = float64(*config.Temperature)
	}
	if config.TopP != nil {
		req.HasTopP = true
		req.TopP = float64(*config.TopP)
	}
	if config.Seed != nil {
		req.HasSeed = true
		req.Seed = int64(*config.Seed)
	}
	if len(config.StopSequences) > 0 {
		req.StopSequences = config.StopSequences
	}
	return req
}

// mapGenerateContentResponse converts a Google GenAI GenerateContentResponse
// to the provider-neutral otelgenai.Response. Only metadata is mapped;
// raw content is never extracted.
func mapGenerateContentResponse(r *genai.GenerateContentResponse) otelgenai.Response {
	if r == nil {
		return otelgenai.Response{}
	}
	resp := otelgenai.Response{
		Model: r.ModelVersion,
		ID:    r.ResponseID,
	}
	// Collect non-empty candidate finish reasons, deduplicated in
	// candidate order.
	seen := make(map[string]bool)
	for _, c := range r.Candidates {
		if c == nil {
			continue
		}
		fr := string(c.FinishReason)
		if fr == "" || seen[fr] {
			continue
		}
		seen[fr] = true
		resp.FinishReasons = append(resp.FinishReasons, fr)
	}
	resp.Usage = mapUsageMetadata(r.UsageMetadata)
	return resp
}

// mapUsageMetadata converts Google GenAI UsageMetadata to the
// provider-neutral otelgenai.Usage.
func mapUsageMetadata(u *genai.GenerateContentResponseUsageMetadata) otelgenai.Usage {
	if u == nil {
		return otelgenai.Usage{}
	}
	return otelgenai.Usage{
		InputTokens:     int64(u.PromptTokenCount),
		OutputTokens:    int64(u.CandidatesTokenCount),
		CacheReadTokens: int64(u.CachedContentTokenCount),
		ReasoningTokens: int64(u.ThoughtsTokenCount),
	}
}

// HasGeneratedOutput reports whether the response contains at least one
// non-empty generated output part. It inspects Candidates[].Content.Parts[]
// for non-empty text parts only. This prevents metadata-only responses
// (usage, model version, finish reason) from being treated as output chunks
// for streaming timing purposes.
//
// v0.2 constraint: only text parts are counted as generated output.
// Non-text parts (inline data/images, function calls, function responses,
// code execution results, file data, etc.) are intentionally NOT counted
// in v0.2. This is a deliberate narrowing to avoid over-claiming output
// detection for modalities that may require different timing semantics.
// Future versions may expand this predicate to cover additional generated
// part types.
func HasGeneratedOutput(resp *genai.GenerateContentResponse) bool {
	if resp == nil {
		return false
	}
	for _, c := range resp.Candidates {
		if c == nil || c.Content == nil {
			continue
		}
		for _, p := range c.Content.Parts {
			if p == nil {
				continue
			}
			if p.Text != "" {
				return true
			}
		}
	}
	return false
}
