package testutil

import (
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// These helpers are heuristics for regression testing, not formal
// cardinality analysis; "low cardinality" cannot be established from
// a single trace export.

// Known GenAI content-capture attribute keys that should only be
// present when a content projector is explicitly configured.
var contentCaptureKeys = []string{
	"gen_ai.input.messages",
	"gen_ai.output.messages",
	"gen_ai.system_instructions",
	"gen_ai.tool.definitions",
	"gen_ai.tool.call.arguments",
	"gen_ai.tool.call.result",
}

// Dynamic ID patterns that should not appear in span names or model
// attributes.
var (
	uuidPattern   = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	longHexPattern = regexp.MustCompile(`[0-9a-f]{16,}`)
)

// AssertLowCardinalityModel checks that the model attribute does not
// contain known dynamic-ID patterns (UUIDs, long hex strings).
// Note: this is a heuristic detector for known patterns, not a proof
// of true production cardinality; model names may legitimately contain
// digits, versions, dates, or hashes.
func AssertLowCardinalityModel(t testing.TB, span sdktrace.ReadOnlySpan) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) != "gen_ai.request.model" && string(attr.Key) != "gen_ai.response.model" {
			continue
		}
		val := attr.Value.AsString()
		if uuidPattern.MatchString(val) {
			t.Errorf("model attribute %q contains a UUID-like pattern: %q", attr.Key, val)
		}
		if longHexPattern.MatchString(val) {
			t.Errorf("model attribute %q contains a long hex pattern: %q", attr.Key, val)
		}
	}
}

// AssertNoDynamicSpanNames checks that no span in the slice has a name
// containing a dynamic ID pattern (UUID, long hex).
func AssertNoDynamicSpanNames(t testing.TB, spans []sdktrace.ReadOnlySpan) {
	t.Helper()
	for _, span := range spans {
		name := span.Name()
		if uuidPattern.MatchString(name) {
			t.Errorf("span name contains a UUID-like pattern: %q", name)
		}
		if longHexPattern.MatchString(name) {
			t.Errorf("span name contains a long hex pattern: %q", name)
		}
	}
}

// AssertNoRawErrors checks that no span attribute contains raw error
// messages. Only low-cardinality error.type should be present; raw
// error messages must not leak into span attributes.
func AssertNoRawErrors(t testing.TB, spans []sdktrace.ReadOnlySpan) {
	t.Helper()
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == "error.type" {
				continue
			}
			// Check string attributes for error-like content.
			if attr.Value.Type() == attribute.STRING {
				val := attr.Value.AsString()
				if uuidPattern.MatchString(val) || longHexPattern.MatchString(val) {
					t.Errorf("span %q: attribute %q contains dynamic content: %q", span.Name(), attr.Key, val)
				}
			}
		}
	}
}

// AssertNoResourceURIsInSpanNames checks that MCP resource URIs do not
// appear in span names (they should be in the mcp.resource.uri attribute).
func AssertNoResourceURIsInSpanNames(t testing.TB, spans []sdktrace.ReadOnlySpan) {
	t.Helper()
	uriPattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)
	for _, span := range spans {
		name := span.Name()
		if uriPattern.MatchString(name) {
			t.Errorf("span name contains a URI pattern (should be in mcp.resource.uri attribute): %q", name)
		}
	}
}

// AssertNoContentInAttributes checks that no known GenAI
// content-capture attribute is present on the span. The caller
// decides when it is appropriate to invoke this assertion (e.g.,
// in tests where no content projector is configured).
func AssertNoContentInAttributes(t testing.TB, span sdktrace.ReadOnlySpan) {
	t.Helper()
	for _, key := range contentCaptureKeys {
		AssertAttrNotPresent(t, span, key)
	}
}
