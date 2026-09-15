package otelgenai

// This file exports the well-known gen_ai.operation.name and gen_ai.system
// values that framework adapters need without depending on the internal
// semconv package. Only the well-known string values are exported; the
// underlying attribute keys remain internal so adapters cannot construct
// arbitrary attributes outside the constrained adapter API.

// Well-known gen_ai.operation.name values. Use these with Request.Operation,
// AgentRequest (implicitly via StartAgent), ToolRequest (implicitly via
// StartTool), and the adapter-facing semantic helpers.
const (
	// OperationChat is the operation name for chat-completion-style
	// inference (OpenAI Chat Completions, Anthropic Messages).
	OperationChat Operation = "chat"
	// OperationGenerateContent is the operation name for
	// generate-content-style inference (Google GenAI, Google ADK).
	OperationGenerateContent Operation = "generate_content"
	// OperationTextCompletion is the operation name for legacy
	// text-completion inference.
	OperationTextCompletion Operation = "text_completion"
	// OperationEmbeddings is the operation name for embedding inference.
	OperationEmbeddings Operation = "embeddings"
	// OperationInvokeAgent is the operation name for local agent
	// invocations.
	OperationInvokeAgent Operation = "invoke_agent"
	// OperationExecuteTool is the operation name for tool executions.
	OperationExecuteTool Operation = "execute_tool"
)

// Well-known gen_ai.system values. Use these with Request.Provider and the
// adapter-facing semantic helpers.
const (
	// SystemOpenAI is the gen_ai.system value for OpenAI and
	// OpenAI-compatible providers using the official OpenAI SDK.
	SystemOpenAI = "openai"
	// SystemAnthropic is the gen_ai.system value for Anthropic.
	SystemAnthropic = "anthropic"
)
