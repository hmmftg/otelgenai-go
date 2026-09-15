// Package adk provides Google ADK Go instrumentation through the ADK
// plugin system, reusing ADK-native semantic spans and emitting
// existing otelgenai metrics through constrained core APIs.
//
// # Architecture
//
// The adapter is a separate Go module pinned to
// google.golang.org/adk/v2 v2.3.0. It translates ADK plugin callbacks
// into the existing otelgenai semantic model without introducing ADK
// types into the core package and without creating duplicate
// invoke_agent, generate_content, execute_tool, or invoke_node spans.
//
// ADK v2.3.0 owns the framework semantic spans:
//
//	invoke_agent
//	  ├── generate_content
//	  │     └── provider adapter span (optional)
//	  └── execute_tool
//	        └── MCP/lower-level operation (optional)
//
// The adapter creates none of these semantic spans. It observes
// ADK's invoke_agent span without mutation, never mutates the
// generate_content span, and may synchronously augment the active
// execute_tool span with adapter-owned attributes and final error/status.
//
// # Callback Identity
//
// ADK v2.3.0 plugin callback wrappers expose InvocationID(), Branch(),
// AgentName(), and SessionID(). They do not support Path(), RunID(), or
// FunctionCallID(). The adapter identity contract is:
//
//	agentKey = InvocationID + Branch + AgentName
//
// This is an adapter callback-execution identity, not a claim that ADK
// exposes a universal agent identifier.
//
// # Correlated Events
//
// In addition to metrics, the adapter emits correlated OTel log-based
// events through the instrumenter's logger:
//
//	gen_ai.client.inference.operation.details   (upstream standard, opt-in)
//	otelgenai.execute_tool.operation.details    (repository-owned, opt-in)
//	otelgenai.agent.model.error_observed        (repository-owned)
//	otelgenai.agent.tool.error_observed         (repository-owned)
//	otelgenai.agent.continued_after_error       (repository-owned)
//
// Content-bearing events carry only projector-controlled content and
// are emitted only when a ContentProjector is configured on the
// instrumenter and the logger accepts records. The standard inference
// event additionally requires a gen_ai.provider.name resolved through
// WithInferenceProvider or WithInferenceProviderResolver; without one
// the event is not emitted. Because ADK v2.3.0 ends the
// generate_content span before AfterModel callbacks run, inference
// details events are correlated to the enclosing agent trace context,
// not the ended model span.
//
// Occurrence events carry no content and record only that something
// happened: continued_after_error means the agent started another model
// or tool call after a previously observed error; it makes no claim
// about retry, fallback, or recovery.
//
// Event timestamps are occurrence times captured at callback entry, not
// export times. The ADK session ID is reported as
// gen_ai.conversation.id on events; it is never added to metrics.
//
// # Tool Identity
//
// Tool state is keyed by the composite {TraceID, SpanID} of the active
// execute_tool span. A SpanID alone is only unique within its trace;
// the composite key prevents cross-trace collisions in concurrent runs.
// Invalid span contexts fail closed: no state, no metrics, no parent
// counter increment.
//
// # Non-Interception
//
// Every intercept-capable adapter callback is observational/non-intercepting:
// it returns (nil, nil), never a replacement response/result/content,
// and never an error solely for telemetry purposes. AfterRunCallback is
// teardown-only and has no return value.
//
// # Cross-Plugin Ordering
//
// Register the otelgenai plugin first for strongest terminal-metric
// completeness. If a later plugin intercepts Before* after otelgenai
// created state, the native operation may be skipped and no matching
// terminal callback arrives. AfterRunCallback performs invocation-scoped
// cleanup of abandoned state without synthesizing terminal metrics.
// If otelgenai is registered after a plugin that intercepts After*,
// terminal telemetry may be missing; this is a documented limitation.
//
// # Limitations
//
//   - Unknown/streaming tool paths without the ordinary callback sequence
//     cannot be fully counted.
//   - agenttool-created nested runners may not inherit PluginConfig in
//     v2.3.0 (ADK issue #669).
//   - No workflow-node (invoke_node) augmentation; ADK already creates
//     it and exposes no public node callback.
//   - No default content capture; prompts, responses, tool arguments,
//     and tool results are never recorded unless a ContentProjector is
//     configured and the logger accepts events.
//   - ADK's native opt-in content capture
//     (OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT) is outside
//     the adapter's projector-control boundary and may coexist with
//     adapter events.
package adk
