package otelgenai

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// InstrumentationFailure is a bounded, low-cardinality enumeration of
// internal instrumentation failures that framework adapters may report
// through [Instrumenter.ReportInstrumentationFailure]. It never carries
// raw errors, panic values, prompts, results, or content.
type InstrumentationFailure string

const (
	// InstrumentationFailureModelStateConflict indicates that a model
	// operation was started while another was already active for the
	// same agent key. The existing state is preserved.
	InstrumentationFailureModelStateConflict InstrumentationFailure = "model.state.conflict"
	// InstrumentationFailureToolSystemResolverPanic indicates that a
	// tool system resolver panicked. The gen_ai.system attribute is
	// omitted and execution continues.
	InstrumentationFailureToolSystemResolverPanic InstrumentationFailure = "tool_system.resolver.panic"
	// InstrumentationFailureInvalidToolSpan indicates that a tool
	// lifecycle callback was received without a valid active
	// execute_tool span context. Tool lifecycle state and metrics are
	// not emitted.
	InstrumentationFailureInvalidToolSpan InstrumentationFailure = "tool.span.invalid"
	// InstrumentationFailureInvalidMetricValue indicates that a metric
	// recording method was called with an invalid value (e.g. negative
	// duration or negative count). The measurement is omitted.
	InstrumentationFailureInvalidMetricValue InstrumentationFailure = "metric.value.invalid"
	// InstrumentationFailureProviderResolverPanic indicates that an
	// inference provider-name resolver panicked. The inference-details
	// event is not emitted and execution continues.
	InstrumentationFailureProviderResolverPanic InstrumentationFailure = "provider.resolver.panic"
)

// instrumentationFailureToDiagnostic maps each bounded failure to the
// safety.Diagnostic stage/reason pair used by the configured handler.
func instrumentationFailureToDiagnostic(f InstrumentationFailure) safety.Diagnostic {
	switch f {
	case InstrumentationFailureModelStateConflict:
		return safety.Diagnostic{Stage: "model", Reason: safety.ReasonModelStateConflict}
	case InstrumentationFailureToolSystemResolverPanic:
		return safety.Diagnostic{Stage: "tool_system", Reason: safety.ReasonToolSystemResolverPanic}
	case InstrumentationFailureInvalidToolSpan:
		return safety.Diagnostic{Stage: "tool", Reason: safety.ReasonInvalidToolSpan}
	case InstrumentationFailureInvalidMetricValue:
		return safety.Diagnostic{Stage: "metric", Reason: safety.ReasonInvalidMetricValue}
	case InstrumentationFailureProviderResolverPanic:
		return safety.Diagnostic{Stage: "provider", Reason: safety.ReasonProviderResolverPanic}
	default:
		return safety.Diagnostic{Stage: "adapter", Reason: safety.ReasonUnknown}
	}
}

// ClassifyError maps an error to a low-cardinality ErrorType using the
// configured classifier. It is panic-isolated: if the classifier panics,
// a diagnostic is reported and ErrorTypeUnknown is returned for non-nil
// errors. ClassifyError(nil) returns ErrorTypeNone.
//
// Framework adapters use this method instead of the private
// classifyError to ensure consistent cross-adapter behavior without
// accessing private instrumenter fields.
func (in *Instrumenter) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeNone
	}
	if in.cfg.disabled || in.classifier == nil {
		return ErrorTypeUnknown
	}
	var et ErrorType
	panicErr := safety.GuardedCall(func() error {
		et = in.classifier(err)
		return nil
	})
	if panicErr != nil {
		safety.GuardedDiagnostic(in.diag, safety.Diagnostic{
			Stage:  "classifier",
			Reason: safety.ReasonClassifierPanic,
		})
		return ErrorTypeUnknown
	}
	if et == ErrorTypeNone {
		return ErrorTypeUnknown
	}
	return et
}

// ReportInstrumentationFailure reports a bounded internal instrumentation
// failure through the configured diagnostic handler. It accepts only the
// predefined InstrumentationFailure enum values and never carries raw
// errors, panic values, prompts, results, or content. When the
// instrumenter is disabled or no diagnostic handler is configured, this
// is a no-op.
func (in *Instrumenter) ReportInstrumentationFailure(f InstrumentationFailure) {
	if in.cfg.disabled {
		return
	}
	safety.GuardedDiagnostic(in.diag, instrumentationFailureToDiagnostic(f))
}

// ApplySpanOutcome classifies err through the configured (panic-isolated)
// ErrorClassifier and applies the low-cardinality gen_ai error.type
// attribute and OTel status to the active span in ctx. On nil err it sets
// OK status. When instrumentation is disabled, the active span is not
// recording, or the span context is invalid, this is a no-op.
//
// Framework adapters call this inside native semantic-span callbacks to
// stamp the final outcome of an operation whose span is owned by the
// framework rather than by otelgenai-go. It never records raw error
// messages, panic values, or content.
func (in *Instrumenter) ApplySpanOutcome(ctx context.Context, err error) {
	if in.cfg.disabled {
		return
	}
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return
	}
	if err != nil {
		et := in.ClassifyError(err)
		span.SetAttributes(attribute.String(semconv.AttrErrorType, string(et)))
		span.SetStatus(codes.Error, "")
		return
	}
	span.SetStatus(codes.Ok, "")
}

// ToolSpanAttributes carries the semantic attributes a framework adapter
// wants applied to an active execute_tool span. Only well-known,
// low-cardinality fields are accepted; arbitrary attribute keys are not
// exposed.
type ToolSpanAttributes struct {
	// Type is the gen_ai.tool.type value. When empty, "function" is used.
	Type string
	// System is the gen_ai.system value identifying the provider. When
	// empty, no gen_ai.system attribute is set.
	System string
}

// AugmentToolSpan applies the semantic ToolSpanAttributes to the active
// execute_tool span in ctx. It sets gen_ai.tool.type (defaulting to
// "function" when empty) and gen_ai.system (only when non-empty). When
// instrumentation is disabled, the active span is not recording, or the
// span context is invalid, this is a no-op.
//
// Framework adapters call this inside native tool-span callbacks to
// decorate a framework-owned span with canonical GenAI attributes
// without gaining access to arbitrary attribute construction.
func (in *Instrumenter) AugmentToolSpan(ctx context.Context, attrs ToolSpanAttributes) {
	if in.cfg.disabled {
		return
	}
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return
	}
	toolType := attrs.Type
	if toolType == "" {
		toolType = semconv.ToolTypeFunction
	}
	span.SetAttributes(attribute.String(semconv.AttrGenAIToolType, toolType))
	if attrs.System != "" {
		span.SetAttributes(attribute.String(semconv.AttrGenAISystem, attrs.System))
	}
}

// RecordInferenceUsage records token usage metrics for an inference
// operation using the existing client.token.usage instrument. Only
// positive input and output token components produce measurements,
// matching existing core behavior. Cache-read, cache-write, and
// reasoning values remain valid Usage fields but do not produce
// token-type metric measurements in v0.5.
//
// The system, model, and operation arguments are the canonical metric
// dimensions; the root owns attribute construction, filtering, and
// cardinality enforcement. When the instrumenter is disabled, this is
// a no-op.
func (in *Instrumenter) RecordInferenceUsage(
	ctx context.Context,
	usage Usage,
	system, model string,
	operation Operation,
) {
	if in.cfg.disabled || string(operation) == "" || model == "" {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, string(operation)),
		attribute.String(semconv.AttrGenAISystem, system),
		attribute.String(semconv.AttrGenAIRequestModel, model),
	}
	if usage.InputTokens > 0 {
		in.metrics.clientTokenUsage.Record(ctx, usage.InputTokens,
			metric.WithAttributes(append(attrs,
				attribute.String(semconv.AttrGenAITokenType, semconv.TokenTypeInput))...),
		)
	}
	if usage.OutputTokens > 0 {
		in.metrics.clientTokenUsage.Record(ctx, usage.OutputTokens,
			metric.WithAttributes(append(attrs,
				attribute.String(semconv.AttrGenAITokenType, semconv.TokenTypeOutput))...),
		)
	}
}

// RecordInferenceDuration records the duration of an inference operation
// using the existing client.operation.duration instrument. Duration < 0
// is omitted and reported as an instrumentation failure; duration == 0
// is recorded. When the instrumenter is disabled, this is a no-op.
func (in *Instrumenter) RecordInferenceDuration(
	ctx context.Context,
	duration time.Duration,
	system, model string,
	operation Operation,
) {
	if in.cfg.disabled || string(operation) == "" || model == "" {
		return
	}
	secs := duration.Seconds()
	if secs < 0 {
		in.ReportInstrumentationFailure(InstrumentationFailureInvalidMetricValue)
		return
	}
	in.metrics.clientOperationDuration.Record(ctx, secs,
		metric.WithAttributes(
			attribute.String(semconv.AttrGenAIOperationName, string(operation)),
			attribute.String(semconv.AttrGenAISystem, system),
			attribute.String(semconv.AttrGenAIRequestModel, model),
		),
	)
}

// RecordAgentDuration records the duration of an agent invocation using
// the existing invoke_agent.duration instrument. Duration < 0 is omitted
// and reported as an instrumentation failure; duration == 0 is recorded.
// When the instrumenter is disabled, this is a no-op.
func (in *Instrumenter) RecordAgentDuration(
	ctx context.Context,
	duration time.Duration,
	name string,
) {
	if in.cfg.disabled {
		return
	}
	secs := duration.Seconds()
	if secs < 0 {
		in.ReportInstrumentationFailure(InstrumentationFailureInvalidMetricValue)
		return
	}
	var attrs []attribute.KeyValue
	if name != "" {
		attrs = []attribute.KeyValue{
			attribute.String(semconv.AttrGenAIAgentName, name),
		}
	}
	in.metrics.invokeAgentDuration.Record(ctx, secs,
		metric.WithAttributes(attrs...),
	)
}

// RecordAgentInferenceCalls records the number of inference calls made
// by an agent invocation using the existing invoke_agent.inference_calls
// instrument. Zero is recorded once, matching existing agent metric
// behavior. Negative counts are omitted and reported as an
// instrumentation failure. When the instrumenter is disabled, this is
// a no-op.
func (in *Instrumenter) RecordAgentInferenceCalls(
	ctx context.Context,
	count int64,
	name string,
) {
	if in.cfg.disabled {
		return
	}
	if count < 0 {
		in.ReportInstrumentationFailure(InstrumentationFailureInvalidMetricValue)
		return
	}
	var attrs []attribute.KeyValue
	if name != "" {
		attrs = []attribute.KeyValue{
			attribute.String(semconv.AttrGenAIAgentName, name),
		}
	}
	in.metrics.invokeAgentInferenceCalls.Record(ctx, count,
		metric.WithAttributes(attrs...),
	)
}

// RecordAgentToolCalls records the number of tool calls made by an agent
// invocation using the existing invoke_agent.tool_calls instrument. Zero
// is recorded once, matching existing agent metric behavior. Negative
// counts are omitted and reported as an instrumentation failure. When
// the instrumenter is disabled, this is a no-op.
func (in *Instrumenter) RecordAgentToolCalls(
	ctx context.Context,
	count int64,
	name string,
) {
	if in.cfg.disabled {
		return
	}
	if count < 0 {
		in.ReportInstrumentationFailure(InstrumentationFailureInvalidMetricValue)
		return
	}
	var attrs []attribute.KeyValue
	if name != "" {
		attrs = []attribute.KeyValue{
			attribute.String(semconv.AttrGenAIAgentName, name),
		}
	}
	in.metrics.invokeAgentToolCalls.Record(ctx, count,
		metric.WithAttributes(attrs...),
	)
}

// RecordToolDuration records the duration of a tool execution using the
// existing execute_tool.duration instrument. Duration < 0 is omitted and
// reported as an instrumentation failure; duration == 0 is recorded. When
// the instrumenter is disabled, this is a no-op.
func (in *Instrumenter) RecordToolDuration(
	ctx context.Context,
	duration time.Duration,
	name string,
) {
	if in.cfg.disabled {
		return
	}
	secs := duration.Seconds()
	if secs < 0 {
		in.ReportInstrumentationFailure(InstrumentationFailureInvalidMetricValue)
		return
	}
	var attrs []attribute.KeyValue
	if name != "" {
		attrs = []attribute.KeyValue{
			attribute.String(semconv.AttrGenAIToolName, name),
		}
	}
	in.metrics.executeToolDuration.Record(ctx, secs,
		metric.WithAttributes(attrs...),
	)
}
