package adk

import (
	"encoding/json"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/hmmftg/otelgenai-go"
)

// This file normalizes ADK/genai content into otelgenai's canonical
// ContentValue forms. Normalization only happens when content-bearing
// events are enabled; the produced values are always passed through
// the configured ContentProjector before emission. Unsupported part
// kinds (thoughts, inline data, file data, code execution, provider
// tool calls) are omitted rather than mapped into a wrong shape.

// projectSystemInstructions maps req.Config.SystemInstruction into a
// canonical projection. Returns an invalid ProjectedContent when there
// is nothing representable or the projector drops the value.
func (p *Plugin) projectSystemInstructions(req *model.LLMRequest) otelgenai.ProjectedContent {
	if req == nil || req.Config == nil || req.Config.SystemInstruction == nil {
		return otelgenai.ProjectedContent{}
	}
	parts := mapParts(req.Config.SystemInstruction.Parts)
	if len(parts) == 0 {
		return otelgenai.ProjectedContent{}
	}
	return p.instr.ProjectContent(otelgenai.ContentValue{
		Kind:                   otelgenai.ContentKindSystemInstructions,
		SystemInstructionParts: parts,
	})
}

// projectInputMessages maps req.Contents into a canonical projection.
// Messages are kept in order; a turn whose parts are all
// unrepresentable is kept with an empty parts list rather than
// dropped, so a consumer sees a turn it cannot render instead of a
// gap in the conversation.
func (p *Plugin) projectInputMessages(req *model.LLMRequest) otelgenai.ProjectedContent {
	if req == nil || len(req.Contents) == 0 {
		return otelgenai.ProjectedContent{}
	}
	msgs := make([]otelgenai.Message, 0, len(req.Contents))
	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		msgs = append(msgs, otelgenai.Message{
			Role:  schemaRole(c),
			Parts: mapParts(c.Parts),
		})
	}
	if len(msgs) == 0 {
		return otelgenai.ProjectedContent{}
	}
	return p.instr.ProjectContent(otelgenai.ContentValue{
		Kind:     otelgenai.ContentKindInputMessages,
		Messages: msgs,
	})
}

// projectOutputMessages maps a terminal LLMResponse into a canonical
// projection. ADK carries a single candidate, so there is always
// exactly one output message; a suppressed or empty candidate is
// recorded with an empty parts list.
func (p *Plugin) projectOutputMessages(resp *model.LLMResponse) otelgenai.ProjectedContent {
	var parts []otelgenai.MessagePart
	if resp != nil && resp.Content != nil {
		parts = mapParts(resp.Content.Parts)
	}
	return p.instr.ProjectContent(otelgenai.ContentValue{
		Kind: otelgenai.ContentKindOutputMessages,
		Messages: []otelgenai.Message{{
			Role:         "assistant",
			Parts:        parts,
			FinishReason: schemaFinishReason(resp, hasToolCall(parts)),
		}},
	})
}

// projectToolArguments encodes tool call arguments and projects them.
// The JSON string form is the canonical representation; encoding
// failures produce an invalid projection with no fallback.
func (p *Plugin) projectToolArguments(args map[string]any) otelgenai.ProjectedContent {
	raw, err := json.Marshal(args)
	if err != nil {
		return otelgenai.ProjectedContent{}
	}
	return p.instr.ProjectContent(otelgenai.ContentValue{
		Kind:              otelgenai.ContentKindToolCallArguments,
		ToolCallArguments: string(raw),
	})
}

// projectToolResult encodes a tool call result and projects it.
func (p *Plugin) projectToolResult(result map[string]any) otelgenai.ProjectedContent {
	raw, err := json.Marshal(result)
	if err != nil {
		return otelgenai.ProjectedContent{}
	}
	return p.instr.ProjectContent(otelgenai.ContentValue{
		Kind:           otelgenai.ContentKindToolCallResult,
		ToolCallResult: string(raw),
	})
}

// mapParts converts genai parts to canonical message parts, dropping
// kinds the canonical model does not represent. Thought parts are
// omitted: they are model reasoning, not conversation content.
func mapParts(ps []*genai.Part) []otelgenai.MessagePart {
	out := make([]otelgenai.MessagePart, 0, len(ps))
	for _, part := range ps {
		if mp, ok := mapPart(part); ok {
			out = append(out, mp)
		}
	}
	return out
}

// mapPart converts one genai part. Structured variants are matched
// before text.
func mapPart(part *genai.Part) (otelgenai.MessagePart, bool) {
	switch {
	case part == nil:
		return otelgenai.MessagePart{}, false
	case part.FunctionCall != nil:
		mp := otelgenai.MessagePart{
			Type: otelgenai.MessagePartToolCall,
			ID:   part.FunctionCall.ID,
			Name: part.FunctionCall.Name,
		}
		if raw, err := json.Marshal(part.FunctionCall.Args); err == nil {
			mp.Arguments = raw
		}
		return mp, true
	case part.FunctionResponse != nil:
		mp := otelgenai.MessagePart{
			Type: otelgenai.MessagePartToolCallResponse,
			ID:   part.FunctionResponse.ID,
			Name: part.FunctionResponse.Name,
		}
		if raw, err := json.Marshal(part.FunctionResponse.Response); err == nil {
			mp.Response = raw
		}
		return mp, true
	case part.Text != "" && !part.Thought:
		return otelgenai.MessagePart{
			Type:    otelgenai.MessagePartText,
			Content: part.Text,
		}, true
	default:
		return otelgenai.MessagePart{}, false
	}
}

// schemaRole maps a genai role onto the schema role enum. A turn
// carrying a tool result is a "tool" message even though genai labels
// it "user". Other roles pass through.
func schemaRole(c *genai.Content) string {
	if c == nil {
		return "user"
	}
	for _, p := range c.Parts {
		if p != nil && (p.FunctionResponse != nil || p.ToolResponse != nil) {
			return "tool"
		}
	}
	switch c.Role {
	case "", genai.RoleUser:
		return "user"
	case genai.RoleModel:
		return "assistant"
	default:
		return c.Role
	}
}

// hasToolCall reports whether any part is a tool call.
func hasToolCall(parts []otelgenai.MessagePart) bool {
	for _, p := range parts {
		if p.Type == otelgenai.MessagePartToolCall {
			return true
		}
	}
	return false
}

// schemaFinishReason maps a response onto the schema finish_reason
// values. A response carrying an error code or interruption reports
// "error"; a stop carrying tool calls reports "tool_call"; unknown
// genai enum values are lowercased, which the schema allows.
func schemaFinishReason(resp *model.LLMResponse, toolCall bool) string {
	if resp == nil {
		return "error"
	}
	if resp.ErrorCode != "" || resp.Interrupted {
		return "error"
	}
	switch resp.FinishReason {
	case "", genai.FinishReasonUnspecified, genai.FinishReasonStop:
		if toolCall {
			return "tool_call"
		}
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	case genai.FinishReasonSafety,
		genai.FinishReasonRecitation,
		genai.FinishReasonLanguage,
		genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII,
		genai.FinishReasonImageSafety,
		genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation:
		return "content_filter"
	default:
		return strings.ToLower(string(resp.FinishReason))
	}
}
