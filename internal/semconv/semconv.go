// Package semconv pins the OpenTelemetry GenAI semantic conventions used by
// otelgenai-go v0.1. The GenAI conventions are still in development, so this
// package is the single source of truth for attribute keys, well-known values,
// span names, instrument names, units, descriptions, and recommended
// histogram boundaries.
//
// Pinned snapshot: GenAI semantic conventions as published by the
// OpenTelemetry Semantic Conventions working group, compatible with the
// go.opentelemetry.io/otel/semconv/v1.30.0 generation. Only the GenAI subset
// relevant to v0.1 is modeled here; missing generated surfaces are defined
// locally and centralized so convention upgrades are deliberate, reviewed
// changes.
package semconv

// Attribute keys for GenAI spans and metrics.
const (
	// System attributes.
	AttrGenAISystem               = "gen_ai.system"
	AttrGenAIRequestModel         = "gen_ai.request.model"
	AttrGenAIResponseModel        = "gen_ai.response.model"
	AttrGenAIResponseID           = "gen_ai.response.id"
	AttrGenAIOperationName        = "gen_ai.operation.name"
	AttrGenAIRequestMaxTokens     = "gen_ai.request.max_tokens"
	AttrGenAIRequestTemperature   = "gen_ai.request.temperature"
	AttrGenAIRequestTopP          = "gen_ai.request.top_p"
	AttrGenAIRequestSeed          = "gen_ai.request.seed"
	AttrGenAIRequestStopSequences = "gen_ai.request.stop_sequences"
	AttrGenAIRequestStreaming     = "gen_ai.request.streaming"
	AttrGenAIServerAddress        = "server.address"
	AttrGenAIServerPort           = "server.port"

	// Usage attributes.
	AttrGenAIUsageInputTokens           = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputTokens          = "gen_ai.usage.output_tokens"
	AttrGenAITokenType                  = "gen_ai.token.type"
	AttrGenAIUsageCacheReadInputTokens  = "gen_ai.usage.cache_read_input_tokens"
	AttrGenAIUsageCacheWriteInputTokens = "gen_ai.usage.cache_write_input_tokens"
	AttrGenAIUsageReasoningTokens       = "gen_ai.usage.reasoning_tokens"

	// Response attributes.
	AttrGenAIResponseFinishReasons    = "gen_ai.response.finish_reasons"
	AttrGenAIResponseTimeToFirstChunk = "gen_ai.response.time_to_first_chunk"

	// Error attributes.
	AttrErrorType = "error.type"

	// Agent attributes.
	AttrGenAIAgentName        = "gen_ai.agent.name"
	AttrGenAIAgentDescription = "gen_ai.agent.description"
	AttrGenAIAgentID          = "gen_ai.agent.id"

	// Tool attributes.
	AttrGenAIToolName   = "gen_ai.tool.name"
	AttrGenAIToolType   = "gen_ai.tool.type"
	AttrGenAIToolCallID = "gen_ai.tool.call.id"

	// Opt-in content attributes (never set by default).
	AttrGenAIInputMessages      = "gen_ai.input.messages"
	AttrGenAIOutputMessages     = "gen_ai.output.messages"
	AttrGenAISystemInstructions = "gen_ai.system_instructions"
	AttrGenAIToolDefinitions    = "gen_ai.tool.definitions"
	AttrGenAIToolCallArguments  = "gen_ai.tool.call.arguments"
	AttrGenAIToolCallResult     = "gen_ai.tool.call.result"

	// Correlation attribute (traces and events only, never metrics).
	AttrGenAIConversationID = "gen_ai.conversation.id"

	// Event-only attributes following the current upstream GenAI event
	// schema. These names differ from the pinned span conventions.
	AttrGenAIProviderName               = "gen_ai.provider.name"
	AttrGenAIRequestStream              = "gen_ai.request.stream"
	AttrGenAIEventUsageCacheReadTokens  = "gen_ai.usage.cache_read.input_tokens"
	AttrGenAIEventUsageCacheWriteTokens = "gen_ai.usage.cache_write.input_tokens"
	AttrGenAIEventUsageReasoningTokens  = "gen_ai.usage.reasoning.output_tokens"
)

// Well-known gen_ai.system values.
const (
	GenAISystemOpenAI    = "openai"
	GenAISystemAnthropic = "anthropic"
)

// Well-known gen_ai.operation.name values.
const (
	OperationChat            = "chat"
	OperationGenerateContent = "generate_content"
	OperationTextCompletion  = "text_completion"
	OperationEmbeddings      = "embeddings"
	OperationInvokeAgent     = "invoke_agent"
	OperationExecuteTool     = "execute_tool"
)

// Well-known gen_ai.token.type values.
const (
	TokenTypeInput  = "input"
	TokenTypeOutput = "output"
)

// Well-known gen_ai.tool.type values.
const (
	ToolTypeFunction = "function"
)

// Event names. Event names are static and event-specific; dynamic
// identity belongs in attributes.
const (
	// EventInferenceOperationDetails is the upstream-standard opt-in
	// event describing a GenAI inference operation's input/output
	// details independently from traces.
	EventInferenceOperationDetails = "gen_ai.client.inference.operation.details"

	// The following are repository-owned experimental events.
	EventToolOperationDetails     = "otelgenai.execute_tool.operation.details"
	EventAgentModelErrorObserved  = "otelgenai.agent.model.error_observed"
	EventAgentToolErrorObserved   = "otelgenai.agent.tool.error_observed"
	EventAgentContinuedAfterError = "otelgenai.agent.continued_after_error"
)

// Span name templates.
const (
	SpanNameInferenceFormat = "%s %s" // operation, request_model
	SpanNameAgentFormat     = "%s %s" // operation, agent_name
	SpanNameAgentDefault    = "invoke_agent"
	SpanNameToolFormat      = "%s %s" // operation, tool_name
)

// Metric instrument names.
const (
	MetricClientOperationDuration           = "gen_ai.client.operation.duration"
	MetricClientTokenUsage                  = "gen_ai.client.token.usage"
	MetricClientOperationTimeToFirstChunk   = "gen_ai.client.operation.time_to_first_chunk"
	MetricClientOperationTimePerOutputChunk = "gen_ai.client.operation.time_per_output_chunk"
	MetricInvokeAgentDuration               = "gen_ai.invoke_agent.duration"
	MetricInvokeAgentInferenceCalls         = "gen_ai.invoke_agent.inference_calls"
	MetricInvokeAgentToolCalls              = "gen_ai.invoke_agent.tool_calls"
	MetricExecuteToolDuration               = "gen_ai.execute_tool.duration"
)

// Metric units.
const (
	UnitSeconds        = "s"
	UnitTokens         = "{token}"
	UnitInferenceCalls = "{inference_call}"
	UnitToolCalls      = "{tool_call}"
)

// Metric descriptions.
const (
	DescClientOperationDuration           = "Duration of GenAI client operations."
	DescClientTokenUsage                  = "Number of tokens used by GenAI client operations."
	DescClientOperationTimeToFirstChunk   = "Time to first output chunk for streaming GenAI operations."
	DescClientOperationTimePerOutputChunk = "Time between output chunks for streaming GenAI operations."
	DescInvokeAgentDuration               = "Duration of GenAI agent invocations."
	DescInvokeAgentInferenceCalls         = "Number of inference calls made by a GenAI agent invocation."
	DescInvokeAgentToolCalls              = "Number of tool calls made by a GenAI agent invocation."
	DescExecuteToolDuration               = "Duration of GenAI tool executions."
)

// DurationBoundaries are recommended explicit histogram boundaries for duration metrics (seconds).
var DurationBoundaries = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}

// TimeToFirstChunkBoundaries are recommended explicit histogram boundaries for time-to-first-chunk (seconds).
var TimeToFirstChunkBoundaries = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// TimePerOutputChunkBoundaries are recommended explicit histogram boundaries for time-per-output-chunk (seconds).
var TimePerOutputChunkBoundaries = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

// TokenUsageBoundaries are recommended explicit histogram boundaries for token usage counts.
var TokenUsageBoundaries = []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576}

// CountBoundaries are recommended explicit histogram boundaries for inference/tool call counts.
var CountBoundaries = []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

// ConventionVersion identifies the pinned semantic-convention snapshot.
const ConventionVersion = "GenAI semantic conventions (development), compatible with semconv/v1.30.0 generation"
