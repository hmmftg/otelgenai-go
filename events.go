package otelgenai

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"

	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// InferenceDetails carries the supported subset of the upstream
// gen_ai.client.inference.operation.details event schema. The event is
// opt-in: it is only emitted when at least one content field holds a
// valid projection produced by ProjectContent.
//
// Unsupported upstream fields (request sampling parameters, prompt
// identity, output type, per-modality usage, server address, etc.) are
// intentionally omitted in this release. The schema is evolving; new
// fields are added deliberately.
type InferenceDetails struct {
	// Operation and Provider are required by the upstream schema.
	Operation Operation
	Provider  string
	// RequestModel and ResponseModel are reported when available.
	RequestModel  string
	ResponseModel string
	// ConversationID correlates the event with a conversation. When
	// empty, the context value set by WithConversationID is used.
	ConversationID string
	// Streaming reports gen_ai.request.stream when true.
	Streaming bool
	// FinishReasons and Usage are reported when present.
	FinishReasons []string
	Usage         Usage
	// Projected content fields. Only values produced by
	// Instrumenter.ProjectContent are honored; at least one must be
	// valid for the event to be emitted.
	SystemInstructions ProjectedContent
	InputMessages      ProjectedContent
	OutputMessages     ProjectedContent
	// ErrorType is reported when the operation ended in error.
	ErrorType ErrorType
	// OccurredAt is the time the operation completed. When zero, the
	// time is captured at emitter entry.
	OccurredAt time.Time
}

// ToolDetails carries the repository-owned
// otelgenai.execute_tool.operation.details event payload. It is emitted
// only when at least one content field holds a valid projection or an
// error type is present.
type ToolDetails struct {
	// ToolName is required.
	ToolName string
	// CallID is reported when the framework provides one.
	CallID string
	// ConversationID correlates the event with a conversation. When
	// empty, the context value set by WithConversationID is used.
	ConversationID string
	// Arguments and Result hold projected tool call content. Result is
	// only reported for successful executions.
	Arguments ProjectedContent
	Result    ProjectedContent
	// ErrorType is reported when the tool execution ended in error.
	ErrorType ErrorType
	// OccurredAt is the time the tool execution completed. When zero,
	// the time is captured at emitter entry.
	OccurredAt time.Time
}

// OccurrenceKind identifies a repository-owned occurrence event.
// Occurrences record that something happened; they never claim retry,
// fallback, or recovery semantics the framework did not provide.
type OccurrenceKind string

// OccurrenceKind values for repository-owned occurrence events.
const (
	// OccurrenceModelErrorObserved records that a model call error was
	// observed by the framework error callback.
	OccurrenceModelErrorObserved OccurrenceKind = semconv.EventAgentModelErrorObserved
	// OccurrenceToolErrorObserved records that a tool call error was
	// observed by the framework error callback.
	OccurrenceToolErrorObserved OccurrenceKind = semconv.EventAgentToolErrorObserved
	// OccurrenceContinuedAfterError records that the agent continued
	// with a subsequent model or tool call after a previously observed
	// error. It makes no claim about retry, fallback, or recovery.
	OccurrenceContinuedAfterError OccurrenceKind = semconv.EventAgentContinuedAfterError
)

// AgentOccurrence describes a point-in-time occurrence within an agent
// run: an observed model or tool error, or continuation after one.
type AgentOccurrence struct {
	// Kind selects the event name; it must be one of the
	// Occurrence* constants.
	Kind OccurrenceKind
	// AgentName is the agent within which the occurrence happened.
	AgentName string
	// Operation is the operation that failed or continued
	// (generate_content or execute_tool).
	Operation Operation
	// ConversationID correlates the event with a conversation. When
	// empty, the context value set by WithConversationID is used.
	ConversationID string
	// ErrorType is the low-cardinality classification of the observed
	// error (for error-observed events) or of the error that preceded
	// continuation.
	ErrorType ErrorType
	// OccurredAt is the time the occurrence happened (callback entry).
	// When zero, the time is captured at emitter entry.
	OccurredAt time.Time
}

// EventsEnabled reports whether the configured logger currently accepts
// event records. It is a cheap preflight for adapters; it does not
// guarantee a later emit will be accepted.
func (in *Instrumenter) EventsEnabled(ctx context.Context) bool {
	if in == nil || in.cfg.disabled || in.logger == nil {
		return false
	}
	return in.logger.Enabled(ctx, otellog.EnabledParameters{
		EventName: semconv.EventInferenceOperationDetails,
	})
}

// ContentEventsEnabled reports whether content-bearing events can be
// emitted: a projector is configured and the logger accepts events.
// Adapters should use this preflight before normalizing raw SDK
// objects into canonical content values.
func (in *Instrumenter) ContentEventsEnabled(ctx context.Context) bool {
	return in.hasProjector() && in.EventsEnabled(ctx)
}

// EmitInferenceDetails emits the upstream-standard
// gen_ai.client.inference.operation.details event correlated to the
// trace context carried by ctx. Emission requires a valid Operation and
// Provider plus at least one valid projected content field; otherwise
// the call is a no-op (an invalid event produces a diagnostic).
func (in *Instrumenter) EmitInferenceDetails(ctx context.Context, d InferenceDetails) {
	if in == nil || in.cfg.disabled {
		return
	}
	if d.Operation == "" || d.Provider == "" {
		in.reportEventFailure(safety.ReasonInvalidEvent)
		return
	}
	hasContent := d.SystemInstructions.Valid() || d.InputMessages.Valid() || d.OutputMessages.Valid()
	if !hasContent {
		return
	}
	if !in.logger.Enabled(ctx, otellog.EnabledParameters{
		EventName: semconv.EventInferenceOperationDetails,
	}) {
		return
	}

	conversationID := d.ConversationID
	if conversationID == "" {
		conversationID, _ = ConversationIDFromContext(ctx)
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, string(d.Operation)),
		attribute.String(semconv.AttrGenAIProviderName, d.Provider),
	}
	if d.RequestModel != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIRequestModel, d.RequestModel))
	}
	if d.ResponseModel != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIResponseModel, d.ResponseModel))
	}
	if conversationID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIConversationID, conversationID))
	}
	if d.Streaming {
		attrs = append(attrs, attribute.Bool(semconv.AttrGenAIRequestStream, true))
	}
	if len(d.FinishReasons) > 0 {
		attrs = append(attrs, attribute.StringSlice(semconv.AttrGenAIResponseFinishReasons, d.FinishReasons))
	}
	if d.Usage.InputTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageInputTokens, d.Usage.InputTokens))
	}
	if d.Usage.OutputTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageOutputTokens, d.Usage.OutputTokens))
	}
	if d.Usage.CacheReadTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIEventUsageCacheReadTokens, d.Usage.CacheReadTokens))
	}
	if d.Usage.CacheWriteTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIEventUsageCacheWriteTokens, d.Usage.CacheWriteTokens))
	}
	if d.Usage.ReasoningTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIEventUsageReasoningTokens, d.Usage.ReasoningTokens))
	}
	if d.ErrorType != "" && d.ErrorType != ErrorTypeNone {
		attrs = append(attrs, attribute.String(semconv.AttrErrorType, string(d.ErrorType)))
	}
	if d.SystemInstructions.Valid() {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(semconv.AttrGenAISystemInstructions),
			Value: d.SystemInstructions.logValue(),
		})
	}
	if d.InputMessages.Valid() {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(semconv.AttrGenAIInputMessages),
			Value: d.InputMessages.logValue(),
		})
	}
	if d.OutputMessages.Valid() {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(semconv.AttrGenAIOutputMessages),
			Value: d.OutputMessages.logValue(),
		})
	}

	severity := otellog.SeverityInfo
	if d.ErrorType != "" && d.ErrorType != ErrorTypeNone {
		severity = otellog.SeverityError
	}
	in.emitEvent(ctx, semconv.EventInferenceOperationDetails, severity, d.OccurredAt, attrs)
}

// EmitToolDetails emits the repository-owned
// otelgenai.execute_tool.operation.details event correlated to the
// trace context carried by ctx. Emission requires a tool name plus at
// least one valid projected content field or an error type.
func (in *Instrumenter) EmitToolDetails(ctx context.Context, d ToolDetails) {
	if in == nil || in.cfg.disabled {
		return
	}
	if d.ToolName == "" {
		in.reportEventFailure(safety.ReasonInvalidEvent)
		return
	}
	hasContent := d.Arguments.Valid() || d.Result.Valid()
	hasError := d.ErrorType != "" && d.ErrorType != ErrorTypeNone
	if !hasContent && !hasError {
		return
	}
	if !in.logger.Enabled(ctx, otellog.EnabledParameters{
		EventName: semconv.EventToolOperationDetails,
	}) {
		return
	}

	conversationID := d.ConversationID
	if conversationID == "" {
		conversationID, _ = ConversationIDFromContext(ctx)
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, semconv.OperationExecuteTool),
		attribute.String(semconv.AttrGenAIToolName, d.ToolName),
	}
	if d.CallID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIToolCallID, d.CallID))
	}
	if conversationID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIConversationID, conversationID))
	}
	if hasError {
		attrs = append(attrs, attribute.String(semconv.AttrErrorType, string(d.ErrorType)))
	}
	if d.Arguments.Valid() {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(semconv.AttrGenAIToolCallArguments),
			Value: d.Arguments.logValue(),
		})
	}
	if d.Result.Valid() {
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(semconv.AttrGenAIToolCallResult),
			Value: d.Result.logValue(),
		})
	}

	severity := otellog.SeverityInfo
	if hasError {
		severity = otellog.SeverityError
	}
	in.emitEvent(ctx, semconv.EventToolOperationDetails, severity, d.OccurredAt, attrs)
}

// EmitAgentOccurrence emits a repository-owned occurrence event
// correlated to the trace context carried by ctx. Occurrence events
// carry no content and therefore do not require a projector.
func (in *Instrumenter) EmitAgentOccurrence(ctx context.Context, o AgentOccurrence) {
	if in == nil || in.cfg.disabled {
		return
	}
	var eventName string
	switch o.Kind {
	case OccurrenceModelErrorObserved:
		eventName = semconv.EventAgentModelErrorObserved
	case OccurrenceToolErrorObserved:
		eventName = semconv.EventAgentToolErrorObserved
	case OccurrenceContinuedAfterError:
		eventName = semconv.EventAgentContinuedAfterError
	default:
		in.reportEventFailure(safety.ReasonInvalidEvent)
		return
	}
	if o.Operation == "" {
		in.reportEventFailure(safety.ReasonInvalidEvent)
		return
	}
	if !in.logger.Enabled(ctx, otellog.EnabledParameters{EventName: eventName}) {
		return
	}

	conversationID := o.ConversationID
	if conversationID == "" {
		conversationID, _ = ConversationIDFromContext(ctx)
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, string(o.Operation)),
	}
	if o.AgentName != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIAgentName, o.AgentName))
	}
	if conversationID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIConversationID, conversationID))
	}
	if o.ErrorType != "" && o.ErrorType != ErrorTypeNone {
		attrs = append(attrs, attribute.String(semconv.AttrErrorType, string(o.ErrorType)))
	}

	severity := otellog.SeverityInfo
	if o.Kind != OccurrenceContinuedAfterError {
		severity = otellog.SeverityError
	}
	in.emitEvent(ctx, eventName, severity, o.OccurredAt, attrs)
}

// emitEvent emits a log record with the given event name. Timestamp is
// the occurrence time; ObservedTimestamp is left to the SDK.
func (in *Instrumenter) emitEvent(ctx context.Context, name string, severity otellog.Severity, occurredAt time.Time, attrs []attribute.KeyValue) {
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	var rec otellog.Record
	rec.SetEventName(name)
	rec.SetTimestamp(occurredAt)
	rec.SetSeverity(severity)
	rec.AddAttributes(attrs...)
	in.logger.Emit(ctx, rec)
}

// reportEventFailure reports a low-cardinality event emission failure.
func (in *Instrumenter) reportEventFailure(reason safety.Reason) {
	safety.GuardedDiagnostic(in.diag, safety.Diagnostic{
		Stage:  "event",
		Reason: reason,
	})
}
