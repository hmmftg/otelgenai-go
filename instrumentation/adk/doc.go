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
// and AgentName(). They do not support Path(), RunID(), or
// FunctionCallID(). The adapter identity contract is:
//
//	agentKey = InvocationID + Branch + AgentName
//
// This is an adapter callback-execution identity, not a claim that ADK
// exposes a universal agent identifier.
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
//     and tool results are never recorded.
package adk
