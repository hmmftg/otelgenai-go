package adk

import (
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
)

// agentStateKey is the adapter callback-execution identity:
// InvocationID + Branch + AgentName.
type agentStateKey struct {
	InvocationID string
	Branch       string
	AgentName    string
}

// toolStateKey is the composite OTel operation identity for tool state.
// A SpanID alone is only unique within its trace; the composite key
// prevents cross-trace collisions in concurrent runs.
type toolStateKey struct {
	TraceID trace.TraceID
	SpanID  trace.SpanID
}

// agentState tracks one agent invocation's lifecycle.
type agentState struct {
	key            agentStateKey
	name           string
	start          time.Time
	inferenceCalls int64
	toolCalls      int64
	terminalized   bool
	// pendingError is the classified error type of the most recently
	// observed child error that has not yet been followed by a
	// continued_after_error occurrence. Only the first pending error is
	// retained; continuation clears it.
	pendingError otelgenai.ErrorType
}

// modelState tracks one active model operation per agent key.
type modelState struct {
	operationID  uint64
	agentKey     agentStateKey
	start        time.Time
	model        string
	usage        otelgenai.Usage
	terminalized bool
	// streaming is set when any partial response was observed.
	streaming bool
	// provider is the resolved gen_ai.provider.name for the standard
	// inference-details event. Empty when no provider is configured.
	provider string
	// Content projections captured at BeforeModel, used by the
	// terminal inference-details event.
	systemInstructions otelgenai.ProjectedContent
	inputMessages      otelgenai.ProjectedContent
}

// toolState tracks one tool execution's lifecycle.
type toolState struct {
	key          toolStateKey
	agentKey     agentStateKey
	invocationID string
	start        time.Time
	name         string
	terminalized bool
	// arguments is the projected tool call arguments captured at
	// BeforeTool for the terminal tool-details event.
	arguments otelgenai.ProjectedContent
}

// registry is a race-safe lifecycle state registry owned by one plugin
// instance. It holds agent, model, and tool state for concurrent runs.
type registry struct {
	mu sync.Mutex

	agents map[agentStateKey]*agentState
	models map[agentStateKey]*modelState
	tools  map[toolStateKey]*toolState

	// modelOps tracks the next model operation ID per agent key.
	modelOps map[agentStateKey]uint64
}

// newRegistry creates a race-safe lifecycle registry.
func newRegistry() *registry {
	return &registry{
		agents:   make(map[agentStateKey]*agentState),
		models:   make(map[agentStateKey]*modelState),
		tools:    make(map[toolStateKey]*toolState),
		modelOps: make(map[agentStateKey]uint64),
	}
}

// agentKeyFrom derives the agent state key from callback-visible identity.
func agentKeyFrom(invocationID, branch, agentName string) agentStateKey {
	return agentStateKey{
		InvocationID: invocationID,
		Branch:       branch,
		AgentName:    agentName,
	}
}

// startAgent creates agent state keyed by agentKey. It does not mutate
// any span. Returns the created state.
func (r *registry) startAgent(key agentStateKey, name string) *agentState {
	r.mu.Lock()
	defer r.mu.Unlock()

	st := &agentState{
		key:   key,
		name:  name,
		start: time.Now(),
	}
	r.agents[key] = st
	return st
}

// getAgent retrieves agent state by key. Returns nil if not found.
func (r *registry) getAgent(key agentStateKey) *agentState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agents[key]
}

// endAgent terminalizes agent state, returns it for metric emission,
// and deletes it from the registry. Returns nil if not found or
// already terminalized.
func (r *registry) endAgent(key agentStateKey) *agentState {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.agents[key]
	if !ok || st.terminalized {
		return nil
	}
	st.terminalized = true
	delete(r.agents, key)
	return st
}

// incrementAgentInference atomically increments the inference call count
// for the captured parent agent.
func (r *registry) incrementAgentInference(key agentStateKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.agents[key]; ok {
		st.inferenceCalls++
	}
}

// incrementAgentTool atomically increments the tool call count for the
// captured parent agent.
func (r *registry) incrementAgentTool(key agentStateKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.agents[key]; ok {
		st.toolCalls++
	}
}

// markPendingError records a classified child error on the agent state
// so the next BeforeModel/BeforeTool can report continued_after_error.
// Only the first pending error is retained until consumed.
func (r *registry) markPendingError(key agentStateKey, et otelgenai.ErrorType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.agents[key]; ok && st.pendingError == "" {
		st.pendingError = et
	}
}

// takePendingError returns and clears the pending error type for the
// agent, reporting whether one was pending.
func (r *registry) takePendingError(key agentStateKey) (otelgenai.ErrorType, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.agents[key]
	if !ok || st.pendingError == "" {
		return "", false
	}
	et := st.pendingError
	st.pendingError = ""
	return et, true
}

// updateModelRequest records the resolved provider and projected
// request content on the active model state.
func (r *registry) updateModelRequest(key agentStateKey, provider string, sys, input otelgenai.ProjectedContent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.models[key]; ok && !st.terminalized {
		st.provider = provider
		st.systemInstructions = sys
		st.inputMessages = input
	}
}

// updateModelUsage records usage and streaming observation on the
// active model state under the registry lock.
func (r *registry) updateModelUsage(key agentStateKey, usage otelgenai.Usage, streaming bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.models[key]; ok && !st.terminalized {
		st.usage = usage
		st.streaming = st.streaming || streaming
	}
}

// setToolArguments records the projected tool call arguments on the
// tool state.
func (r *registry) setToolArguments(key toolStateKey, args otelgenai.ProjectedContent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.tools[key]; ok && !st.terminalized {
		st.arguments = args
	}
}

// startModel creates model state for the given agent key if no active
// model operation exists. Returns the created state and true on success.
// Returns nil and false if a collision occurred (existing state preserved).
func (r *registry) startModel(key agentStateKey, model string) (*modelState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.models[key]; ok && !existing.terminalized {
		return nil, false
	}

	r.modelOps[key]++
	opID := r.modelOps[key]

	st := &modelState{
		operationID: opID,
		agentKey:    key,
		start:       time.Now(),
		model:       model,
	}
	r.models[key] = st
	return st, true
}

// getModel retrieves model state by agent key. Returns nil if not found.
func (r *registry) getModel(key agentStateKey) *modelState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.models[key]
}

// endModel terminalizes model state, returns it for metric emission,
// and deletes it from the registry. Returns nil if not found or
// already terminalized.
func (r *registry) endModel(key agentStateKey) *modelState {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.models[key]
	if !ok || st.terminalized {
		return nil
	}
	st.terminalized = true
	delete(r.models, key)
	return st
}

// startTool creates tool state keyed by the composite {TraceID, SpanID}
// if both components are valid. Returns the created state and true on
// success. Returns nil and false if the span context is invalid.
func (r *registry) startTool(key toolStateKey, agentKey agentStateKey, invocationID, name string) (*toolState, bool) {
	if !key.TraceID.IsValid() || !key.SpanID.IsValid() {
		return nil, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	st := &toolState{
		key:          key,
		agentKey:     agentKey,
		invocationID: invocationID,
		start:        time.Now(),
		name:         name,
	}
	r.tools[key] = st
	return st, true
}

// getTool retrieves tool state by composite key. Returns nil if not found.
func (r *registry) getTool(key toolStateKey) *toolState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tools[key]
}

// endTool terminalizes tool state, returns it for metric emission and
// parent counter increment, and deletes it from the registry. Returns
// nil if not found or already terminalized.
func (r *registry) endTool(key toolStateKey) *toolState {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.tools[key]
	if !ok || st.terminalized {
		return nil
	}
	st.terminalized = true
	delete(r.tools, key)
	return st
}

// cleanupInvocation removes all agent, model, and tool state belonging
// to the given invocation ID. It is used by AfterRunCallback as an
// abandonment cleanup fallback. It does not emit synthetic terminal
// metrics and does not mutate any span.
func (r *registry) cleanupInvocation(invocationID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k := range r.agents {
		if k.InvocationID == invocationID {
			delete(r.agents, k)
		}
	}
	for k := range r.models {
		if k.InvocationID == invocationID {
			delete(r.models, k)
		}
	}
	for k, st := range r.tools {
		if st.invocationID == invocationID {
			delete(r.tools, k)
		}
	}
}
