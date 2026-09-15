package otelgenai

import (
	"bytes"
	"encoding/json"
	"sort"

	"go.opentelemetry.io/otel/attribute"

	"github.com/hmmftg/otelgenai-go/internal/safety"
)

// ContentKind identifies the kind of content being projected. Each kind
// maps to a specific opt-in span attribute.
type ContentKind string

// ContentKind values identify the kind of content being projected. Each
// value maps to a specific opt-in span attribute.
const (
	ContentKindSystemInstructions ContentKind = "system_instructions"
	ContentKindInputMessages      ContentKind = "input_messages"
	ContentKindOutputMessages     ContentKind = "output_messages"
	ContentKindToolDefinitions    ContentKind = "tool_definitions"
	ContentKindToolCallArguments  ContentKind = "tool_call_arguments"
	ContentKindToolCallResult     ContentKind = "tool_call_result"
)

// MessagePartType identifies the type of a canonical message part,
// matching the upstream gen_ai message-part schema.
type MessagePartType string

// MessagePartType values for canonical message parts.
const (
	MessagePartText             MessagePartType = "text"
	MessagePartToolCall         MessagePartType = "tool_call"
	MessagePartToolCallResponse MessagePartType = "tool_call_response"
)

// ContentValue is the canonical, provider-neutral representation of
// GenAI content. It uses simple types that match the convention schemas.
// Provider adapters normalize their SDK types into these models only
// when the relevant content kind is enabled.
type ContentValue struct {
	Kind ContentKind
	// SystemInstructions is set when Kind is ContentKindSystemInstructions.
	SystemInstructions string
	// SystemInstructionParts is set when Kind is
	// ContentKindSystemInstructions and the framework provides
	// structured instruction parts. When non-nil it takes precedence
	// over SystemInstructions.
	SystemInstructionParts []MessagePart
	// Messages is set when Kind is ContentKindInputMessages or
	// ContentKindOutputMessages.
	Messages []Message
	// ToolDefinitions is set when Kind is ContentKindToolDefinitions.
	ToolDefinitions []ToolDefinition
	// ToolCallArguments is set when Kind is ContentKindToolCallArguments.
	// It holds a JSON-serialized representation of the arguments.
	ToolCallArguments string
	// ToolCallResult is set when Kind is ContentKindToolCallResult. It
	// holds a JSON-serialized representation of the result.
	ToolCallResult string
}

// Message is a canonical input or output message. Content is the
// simple text form; Parts carries the structured representation
// matching the upstream message-part schema. When Parts is non-nil it
// takes precedence over Content during serialization.
type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	Parts        []MessagePart `json:"parts,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

// MessagePart is a canonical typed part of a message. Only the fields
// relevant to Type need to be set: Content for MessagePartText; ID,
// Name, and Arguments for MessagePartToolCall; ID, Name, and Response
// for MessagePartToolCallResponse.
type MessagePart struct {
	Type      MessagePartType `json:"type"`
	Content   string          `json:"content,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Response  json.RawMessage `json:"response,omitempty"`
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

// ProjectedContent is an opaque, bounded, validated content projection.
// It can only be produced by [Instrumenter.ProjectContent]; callers
// cannot construct a non-empty value themselves. The value is
// deep-owned: it holds an immutable canonical encoding independent of
// any telemetry representation. Internally it is encoded separately for
// span JSON attributes and for structured Logs API attributes.
type ProjectedContent struct {
	kind ContentKind
	data []byte
}

// Valid reports whether the projection holds validated content. The
// zero value is invalid.
func (p ProjectedContent) Valid() bool {
	return p.data != nil
}

// Kind returns the content kind of the projection, or "" if invalid.
func (p ProjectedContent) Kind() ContentKind {
	return p.kind
}

// spanJSON encodes the projection for a span attribute: a JSON string
// for scalar kinds, the canonical JSON document for structured kinds.
func (p ProjectedContent) spanJSON() string {
	if !p.Valid() {
		return ""
	}
	return string(p.data)
}

// logValue encodes the projection as a structured attribute.Value for
// the Logs API. JSON objects become map values, arrays become slice
// values, and scalars become their corresponding primitive values. For
// JSON-serialized string kinds (tool call arguments/result), the inner
// document is decoded so the emitted value is structured when the
// projection holds a JSON object or array.
func (p ProjectedContent) logValue() attribute.Value {
	if !p.Valid() {
		return attribute.Value{}
	}
	v, ok := decodeJSON(p.data)
	if !ok {
		return attribute.Value{}
	}
	if p.kind == ContentKindToolCallArguments || p.kind == ContentKindToolCallResult {
		if s, isStr := v.(string); isStr {
			if inner, ok2 := decodeJSON([]byte(s)); ok2 {
				if _, isScalar := inner.(json.Number); !isScalar {
					switch inner.(type) {
					case map[string]any, []any:
						return anyToAttributeValue(inner)
					}
				}
			}
			return attribute.StringValue(s)
		}
	}
	return anyToAttributeValue(v)
}

// decodeJSON decodes a JSON document preserving number precision.
func decodeJSON(data []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// anyToAttributeValue converts a decoded JSON value to attribute.Value.
// The input is bounded: it was already validated and size-limited by
// the projection pipeline.
func anyToAttributeValue(v any) attribute.Value {
	switch t := v.(type) {
	case nil:
		return attribute.StringValue("")
	case bool:
		return attribute.BoolValue(t)
	case string:
		return attribute.StringValue(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return attribute.Int64Value(i)
		}
		if f, err := t.Float64(); err == nil {
			return attribute.Float64Value(f)
		}
		return attribute.StringValue(t.String())
	case []any:
		vals := make([]attribute.Value, 0, len(t))
		for _, e := range t {
			vals = append(vals, anyToAttributeValue(e))
		}
		return attribute.SliceValue(vals...)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			if k != "" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		kvs := make([]attribute.KeyValue, 0, len(keys))
		for _, k := range keys {
			kvs = append(kvs, attribute.KeyValue{
				Key:   attribute.Key(k),
				Value: anyToAttributeValue(t[k]),
			})
		}
		return attribute.MapValue(kvs...)
	default:
		return attribute.Value{}
	}
}

// ProjectContent runs the configured content projector over value and
// returns an opaque, bounded projection. It returns an invalid
// ProjectedContent when no projector is configured, the projector fails
// or panics, the projection shape is invalid for its kind, or the
// serialized projection exceeds the configured byte limit. No raw
// fallback is ever emitted; failures produce diagnostics only.
func (in *Instrumenter) ProjectContent(value ContentValue) ProjectedContent {
	if in == nil || in.cfg.disabled {
		return ProjectedContent{}
	}
	return projectContent(in.cfg.contentProjector, in.cfg.projectionLimit, in.diag, value.Kind, value)
}

// project invokes the configured projector for the given content kind
// and value, returning the serialized projection or empty string.
func (in *Instrumenter) project(kind ContentKind, value ContentValue) string {
	return in.ProjectContent(value).spanJSON()
}

// projectContent invokes the projector with panic isolation, validates
// the result, serializes it to JSON, and enforces the byte limit. It
// returns an opaque ProjectedContent or an invalid value if the
// projection was dropped. A diagnostic is reported on any failure.
func projectContent(
	projector ContentProjector,
	limit int,
	diag safety.DiagnosticHandler,
	kind ContentKind,
	value ContentValue,
) ProjectedContent {
	if projector == nil {
		return ProjectedContent{}
	}

	projected, err := safety.GuardedCallValue(func() (ContentValue, error) {
		return projector(value)
	})
	if err != nil {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorError,
		})
		return ProjectedContent{}
	}
	projected.Kind = kind

	if !validateProjectionShape(projected) {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorInvalid,
		})
		return ProjectedContent{}
	}

	data, err := serializeProjection(projected)
	if err != nil {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorBadJSON,
		})
		return ProjectedContent{}
	}

	if len(data) > limit {
		safety.GuardedDiagnostic(diag, safety.Diagnostic{
			Stage:  string(kind),
			Reason: safety.ReasonProjectorOversize,
		})
		return ProjectedContent{}
	}

	return ProjectedContent{kind: kind, data: data}
}

// serializeProjection serializes the projected content value to JSON,
// including only the field relevant to the content kind.
func serializeProjection(v ContentValue) ([]byte, error) {
	switch v.Kind {
	case ContentKindSystemInstructions:
		if v.SystemInstructionParts != nil {
			return json.Marshal(v.SystemInstructionParts)
		}
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
		return validateParts(v.SystemInstructionParts)
	case ContentKindInputMessages, ContentKindOutputMessages:
		for _, m := range v.Messages {
			if m.Role == "" || !validateParts(m.Parts) {
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

// validateParts checks that structured message parts use supported
// types and valid JSON payloads.
func validateParts(parts []MessagePart) bool {
	for _, p := range parts {
		switch p.Type {
		case MessagePartText:
		case MessagePartToolCall:
			if p.Name == "" {
				return false
			}
		case MessagePartToolCallResponse:
		default:
			return false
		}
		if len(p.Arguments) > 0 && !json.Valid(p.Arguments) {
			return false
		}
		if len(p.Response) > 0 && !json.Valid(p.Response) {
			return false
		}
	}
	return true
}
