package otelgenai

import (
	"encoding/json"

	"github.com/hmmftg/otelgenai-go/internal/safety"
)

// ContentKind identifies the kind of content being projected. Each kind
// maps to a specific opt-in span attribute.
type ContentKind string

const (
	ContentKindSystemInstructions ContentKind = "system_instructions"
	ContentKindInputMessages       ContentKind = "input_messages"
	ContentKindOutputMessages      ContentKind = "output_messages"
	ContentKindToolDefinitions     ContentKind = "tool_definitions"
	ContentKindToolCallArguments   ContentKind = "tool_call_arguments"
	ContentKindToolCallResult      ContentKind = "tool_call_result"
)

// ContentValue is the canonical, provider-neutral representation of
// GenAI content. It uses simple types that match the convention schemas.
// Provider adapters normalize their SDK types into these models only
// when the relevant content kind is enabled.
type ContentValue struct {
	Kind     ContentKind
	// SystemInstructions is set when Kind is ContentKindSystemInstructions.
	SystemInstructions string
	// Messages is set when Kind is ContentKindInputMessages or
	// ContentKindOutputMessages.
	Messages []Message
	// ToolDefinitions is set when Kind is ContentKindToolDefinitions.
	ToolDefinitions []ToolDefinition
	// ToolCallArguments is set when Kind is ContentKindToolCallArguments.
	ToolCallArguments string
	// ToolCallResult is set when Kind is ContentKindToolCallResult.
	ToolCallResult string
}

// Message is a canonical input or output message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolDefinition is a canonical tool definition.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ContentProjector receives a canonical ContentValue and returns a
// redacted/filtered canonical ContentValue. The projector is
// responsible for removing PII, secrets, and any content the user does
// not want in telemetry. The core validates the projection against the
// convention schema and a byte limit before emitting it.
//
// If the projector returns an error or panics, the projection is
// dropped with no raw fallback.
type ContentProjector func(ContentValue) (ContentValue, error)

// projectContent invokes the projector with panic isolation, validates
// the result, serializes it to JSON, and enforces the byte limit. It
// returns the serialized JSON string or empty string if the projection
// was dropped. A diagnostic is reported on any failure.
func projectContent(
	projector ContentProjector,
	limit int,
	diag safety.DiagnosticHandler,
	kind ContentKind,
	value ContentValue,
) string {
	if projector == nil {
		return ""
	}

	projected, err := safety.GuardedCallValue(func() (ContentValue, error) {
		return projector(value)
	})
	if err != nil {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorError,
		})
		return ""
	}
	projected.Kind = kind

	data, err := serializeProjection(projected)
	if err != nil {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorBadJSON,
		})
		return ""
	}

	if len(data) > limit {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorOversize,
		})
		return ""
	}

	if !validateProjectionShape(projected) {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorInvalid,
		})
		return ""
	}

	return string(data)
}

// serializeProjection serializes the projected content value to JSON,
// including only the field relevant to the content kind.
func serializeProjection(v ContentValue) ([]byte, error) {
	switch v.Kind {
	case ContentKindSystemInstructions:
		return json.Marshal(v.SystemInstructions)
	case ContentKindInputMessages, ContentKindOutputMessages:
		return json.Marshal(v.Messages)
	case ContentKindToolDefinitions:
		return json.Marshal(v.ToolDefinitions)
	case ContentKindToolCallArguments:
		return json.Marshal(v.ToolCallArguments)
	case ContentKindToolCallResult:
		return json.Marshal(v.ToolCallResult)
	default:
		return nil, errInvalidContentKind
	}
}

// validateProjectionShape checks that the projected value has the
// correct shape for its kind.
func validateProjectionShape(v ContentValue) bool {
	switch v.Kind {
	case ContentKindSystemInstructions:
		return true
	case ContentKindInputMessages, ContentKindOutputMessages:
		for _, m := range v.Messages {
			if m.Role == "" {
				return false
			}
		}
		return true
	case ContentKindToolDefinitions:
		for _, t := range v.ToolDefinitions {
			if t.Name == "" {
				return false
			}
		}
		return true
	case ContentKindToolCallArguments, ContentKindToolCallResult:
		return true
	default:
		return false
	}
}
