package testutil

import (
	"context"
	"math"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// AssertSpanName checks that a span has the expected name.
func AssertSpanName(t testing.TB, span sdktrace.ReadOnlySpan, expected string) {
	t.Helper()
	if span.Name() != expected {
		t.Errorf("span name: got %q, want %q", span.Name(), expected)
	}
}

// AssertSpanKind checks that a span has the expected kind.
func AssertSpanKind(t testing.TB, span sdktrace.ReadOnlySpan, expected oteltrace.SpanKind) {
	t.Helper()
	if span.SpanKind() != expected {
		t.Errorf("span kind: got %v, want %v", span.SpanKind(), expected)
	}
}

// AssertSpanStatus checks that a span has the expected status code.
func AssertSpanStatus(t testing.TB, span sdktrace.ReadOnlySpan, expected codes.Code) {
	t.Helper()
	if span.Status().Code != expected {
		t.Errorf("span status: got %v, want %v", span.Status().Code, expected)
	}
}

// AssertAttr checks that a span has an attribute with the expected key
// and value.
func AssertAttr(t testing.TB, span sdktrace.ReadOnlySpan, key string, expected attribute.Value) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			if attr.Value.Type() == expected.Type() && attr.Value.String() == expected.String() {
				return
			}
			t.Errorf("span attr %q: got %v, want %v", key, attr.Value, expected)
			return
		}
	}
	t.Errorf("span attr %q: not found", key)
}

// AssertAttrNotPresent checks that a span does NOT have an attribute
// with the given key.
func AssertAttrNotPresent(t testing.TB, span sdktrace.ReadOnlySpan, key string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			t.Errorf("span attr %q: should not be present but was found with value %v", key, attr.Value)
			return
		}
	}
}

// AssertNoSentinel checks that no span attribute contains any of the
// sentinel secret strings.
func AssertNoSentinel(t testing.TB, span sdktrace.ReadOnlySpan, sentinels []string) {
	t.Helper()
	for _, attr := range span.Attributes() {
		s := attr.Value.String()
		for _, sentinel := range sentinels {
			if s == sentinel {
				t.Errorf("span attr %q contains sentinel %q", attr.Key, sentinel)
			}
		}
	}
}

// FindSpan returns the first span with the given name, or nil.
func FindSpan(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

// AssertSpan checks span name, kind, and status in one call. Any zero
// value (empty string, SpanKindUnspecified, or codes.Unset) skips the
// corresponding check.
func AssertSpan(t testing.TB, span sdktrace.ReadOnlySpan, name string, kind oteltrace.SpanKind, status codes.Code) {
	t.Helper()
	if name != "" {
		AssertSpanName(t, span, name)
	}
	if kind != oteltrace.SpanKindUnspecified {
		AssertSpanKind(t, span, kind)
	}
	if status != codes.Unset {
		AssertSpanStatus(t, span, status)
	}
}

// AssertNoContentCaptured verifies that all six opt-in content
// attributes are absent from the span.
func AssertNoContentCaptured(t testing.TB, span sdktrace.ReadOnlySpan) {
	t.Helper()
	contentKeys := []string{
		"gen_ai.input.messages",
		"gen_ai.output.messages",
		"gen_ai.system_instructions",
		"gen_ai.tool.definitions",
		"gen_ai.tool.call.arguments",
		"gen_ai.tool.call.result",
	}
	for _, key := range contentKeys {
		AssertAttrNotPresent(t, span, key)
	}
}

// AssertErrorType checks that the span has the expected error.type
// attribute value.
func AssertErrorType(t testing.TB, span sdktrace.ReadOnlySpan, expected string) {
	t.Helper()
	AssertAttr(t, span, "error.type", attribute.StringValue(expected))
}

// AssertStreaming checks that gen_ai.request.streaming matches the
// expected value on the span.
func AssertStreaming(t testing.TB, span sdktrace.ReadOnlySpan, expected bool) {
	t.Helper()
	AssertAttr(t, span, "gen_ai.request.streaming", attribute.BoolValue(expected))
}

// AssertToolCall checks that the span has the expected tool name and
// type attributes.
func AssertToolCall(t testing.TB, span sdktrace.ReadOnlySpan, name, toolType string) {
	t.Helper()
	AssertAttr(t, span, "gen_ai.tool.name", attribute.StringValue(name))
	AssertAttr(t, span, "gen_ai.tool.type", attribute.StringValue(toolType))
}

// AssertTokenUsage checks that the collected metrics contain the
// expected input and output token sums for the given model. The OTel
// SDK metricdata traversal is kept private to this implementation so
// the public assertion API is decoupled from the exact metric
// representation.
func AssertTokenUsage(t testing.TB, rm metricdata.ResourceMetrics, model string, wantInput, wantOutput int64) {
	t.Helper()
	gotInput, gotOutput := sumTokenUsage(rm, model)
	if gotInput != wantInput {
		t.Errorf("token usage input for model %q: got %d, want %d", model, gotInput, wantInput)
	}
	if gotOutput != wantOutput {
		t.Errorf("token usage output for model %q: got %d, want %d", model, gotOutput, wantOutput)
	}
}

// sumTokenUsage traverses the collected metric data to find the
// gen_ai.client.token.usage histogram and sums the exact HistogramDataPoint
// values for the given model, filtered by token type (input/output).
func sumTokenUsage(rm metricdata.ResourceMetrics, model string) (inputSum, outputSum int64) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.client.token.usage" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				continue
			}
			for _, dp := range hist.DataPoints {
				// Filter by request model attribute.
				modelAttr, hasModel := dp.Attributes.Value(attribute.Key("gen_ai.request.model"))
				if !hasModel || modelAttr.AsString() != model {
					continue
				}
				tokenTypeAttr, hasType := dp.Attributes.Value(attribute.Key("gen_ai.token.type"))
				if !hasType {
					continue
				}
				switch tokenTypeAttr.AsString() {
				case "input":
					inputSum += dp.Sum
				case "output":
					outputSum += dp.Sum
				}
			}
		}
	}
	return inputSum, outputSum
}

// AssertAgentHierarchy locates the invoke_agent span with the given
// name, identifies its direct children by parent SpanID, classifies
// children by gen_ai.operation.name, and compares inference and tool
// counts.
func AssertAgentHierarchy(t testing.TB, spans []sdktrace.ReadOnlySpan, agentName string, wantInference, wantTools int) {
	t.Helper()
	agentSpan := FindSpan(spans, "invoke_agent "+agentName)
	if agentSpan == nil {
		t.Fatalf("agent span %q not found", "invoke_agent "+agentName)
	}
	agentSpanID := agentSpan.SpanContext().SpanID()
	var inferenceCount, toolCount int
	for _, s := range spans {
		if s.Parent().SpanID() != agentSpanID {
			continue
		}
		opName := attrString(s, "gen_ai.operation.name")
		switch opName {
		case "chat", "generate_content", "text_completion", "embeddings":
			inferenceCount++
		case "execute_tool":
			toolCount++
		}
	}
	if inferenceCount != wantInference {
		t.Errorf("agent %q inference children: got %d, want %d", agentName, inferenceCount, wantInference)
	}
	if toolCount != wantTools {
		t.Errorf("agent %q tool children: got %d, want %d", agentName, toolCount, wantTools)
	}
}

// attrString returns the string value of the first attribute with the
// given key, or empty string if not found.
func attrString(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

// CollectMetrics is a convenience helper that collects metrics from
// the recorder using context.Background.
func CollectMetrics(t testing.TB, r *Recorder) metricdata.ResourceMetrics {
	t.Helper()
	rm, err := r.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect metrics: %v", err)
	}
	return rm
}

// AssertEstimatedCost checks that the span has a gen_ai.usage.estimated_cost
// attribute matching wantCost (approximate float comparison).
func AssertEstimatedCost(t testing.TB, span sdktrace.ReadOnlySpan, wantCost float64) {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == "gen_ai.usage.estimated_cost" {
			got := attr.Value.AsFloat64()
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Errorf("estimated_cost: got %v, want %v", got, wantCost)
				return
			}
			if math.Abs(got-wantCost) > 1e-9 {
				t.Errorf("estimated_cost: got %v, want %v", got, wantCost)
			}
			return
		}
	}
	t.Errorf("estimated_cost: attribute not found on span")
}

// AssertNoEstimatedCost checks that the span does NOT have a
// gen_ai.usage.estimated_cost attribute (for no-resolver / unknown-model /
// inconsistent-usage cases).
func AssertNoEstimatedCost(t testing.TB, span sdktrace.ReadOnlySpan) {
	t.Helper()
	AssertAttrNotPresent(t, span, "gen_ai.usage.estimated_cost")
}
