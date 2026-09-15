package adk

import (
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestRegistryStartEndAgent(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}

	st := r.startAgent(key, "agent1")
	if st == nil {
		t.Fatal("startAgent returned nil")
	}
	if st.name != "agent1" {
		t.Fatalf("agent name = %q, want %q", st.name, "agent1")
	}

	// Agent should be retrievable.
	if r.getAgent(key) == nil {
		t.Fatal("getAgent returned nil after startAgent")
	}

	// End agent should return state and delete.
	ended := r.endAgent(key)
	if ended == nil {
		t.Fatal("endAgent returned nil")
	}

	// Second end should return nil (already terminalized/deleted).
	if r.endAgent(key) != nil {
		t.Fatal("second endAgent should return nil")
	}

	// Agent should no longer be retrievable.
	if r.getAgent(key) != nil {
		t.Fatal("getAgent returned non-nil after endAgent")
	}
}

func TestRegistryIncrementAgentCounters(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}

	r.startAgent(key, "agent1")
	r.incrementAgentInference(key)
	r.incrementAgentInference(key)
	r.incrementAgentTool(key)

	st := r.getAgent(key)
	if st.inferenceCalls != 2 {
		t.Fatalf("inferenceCalls = %d, want 2", st.inferenceCalls)
	}
	if st.toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want 1", st.toolCalls)
	}
}

func TestRegistryIncrementNonexistentAgent(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	// Should not panic.
	r.incrementAgentInference(key)
	r.incrementAgentTool(key)
}

func TestRegistryStartModelCollision(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}

	st1, ok1 := r.startModel(key, "model1")
	if !ok1 || st1 == nil {
		t.Fatal("first startModel failed")
	}
	if st1.model != "model1" {
		t.Fatalf("model = %q, want %q", st1.model, "model1")
	}

	// Collision: second startModel should fail, preserving existing state.
	st2, ok2 := r.startModel(key, "model2")
	if ok2 || st2 != nil {
		t.Fatal("second startModel should fail on collision")
	}

	// Existing state should be preserved.
	existing := r.getModel(key)
	if existing == nil || existing.model != "model1" {
		t.Fatal("existing model state not preserved")
	}
}

func TestRegistrySequentialModelOperations(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}

	st1, _ := r.startModel(key, "model1")
	if st1.operationID != 1 {
		t.Fatalf("first operationID = %d, want 1", st1.operationID)
	}
	r.endModel(key)

	st2, _ := r.startModel(key, "model2")
	if st2.operationID != 2 {
		t.Fatalf("second operationID = %d, want 2", st2.operationID)
	}
	r.endModel(key)

	st3, _ := r.startModel(key, "model3")
	if st3.operationID != 3 {
		t.Fatalf("third operationID = %d, want 3", st3.operationID)
	}
}

func TestRegistryStartToolRequiresValidSpan(t *testing.T) {
	r := newRegistry()
	agentKey := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	r.startAgent(agentKey, "agent1")

	// Invalid span key (zero TraceID and SpanID).
	invalidKey := toolStateKey{}
	st, ok := r.startTool(invalidKey, agentKey, "inv1", "tool1")
	if ok || st != nil {
		t.Fatal("startTool should fail with invalid span key")
	}

	// Valid span key.
	validKey := toolStateKey{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	}
	st, ok = r.startTool(validKey, agentKey, "inv1", "tool1")
	if !ok || st == nil {
		t.Fatal("startTool failed with valid span key")
	}
	if st.name != "tool1" {
		t.Fatalf("tool name = %q, want %q", st.name, "tool1")
	}
}

func TestRegistryEndTool(t *testing.T) {
	r := newRegistry()
	agentKey := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	r.startAgent(agentKey, "agent1")

	key := toolStateKey{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	}
	r.startTool(key, agentKey, "inv1", "tool1")

	ended := r.endTool(key)
	if ended == nil {
		t.Fatal("endTool returned nil")
	}

	// Second end should return nil.
	if r.endTool(key) != nil {
		t.Fatal("second endTool should return nil")
	}
}

func TestRegistryCleanupInvocationScoped(t *testing.T) {
	r := newRegistry()

	// Create state for two concurrent invocations.
	key1 := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	key2 := agentStateKey{InvocationID: "inv2", Branch: "root", AgentName: "agent2"}
	r.startAgent(key1, "agent1")
	r.startAgent(key2, "agent2")

	modelKey1 := key1
	modelKey2 := key2
	r.startModel(modelKey1, "model1")
	r.startModel(modelKey2, "model2")

	toolKey1 := toolStateKey{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}}
	toolKey2 := toolStateKey{TraceID: trace.TraceID{2}, SpanID: trace.SpanID{2}}
	r.startTool(toolKey1, key1, "inv1", "tool1")
	r.startTool(toolKey2, key2, "inv2", "tool2")

	// Cleanup inv1 only.
	r.cleanupInvocation("inv1")

	// inv1 state should be gone.
	if r.getAgent(key1) != nil {
		t.Fatal("inv1 agent state not cleaned")
	}
	if r.getModel(modelKey1) != nil {
		t.Fatal("inv1 model state not cleaned")
	}
	if r.getTool(toolKey1) != nil {
		t.Fatal("inv1 tool state not cleaned")
	}

	// inv2 state should remain.
	if r.getAgent(key2) == nil {
		t.Fatal("inv2 agent state should remain")
	}
	if r.getModel(modelKey2) == nil {
		t.Fatal("inv2 model state should remain")
	}
	if r.getTool(toolKey2) == nil {
		t.Fatal("inv2 tool state should remain")
	}
}

func TestRegistryConcurrentRunsSameSpanIDDoNotCollide(t *testing.T) {
	r := newRegistry()

	// Two different traces with the same SpanID.
	key1 := toolStateKey{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{42}}
	key2 := toolStateKey{TraceID: trace.TraceID{2}, SpanID: trace.SpanID{42}}

	agentKey1 := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	agentKey2 := agentStateKey{InvocationID: "inv2", Branch: "root", AgentName: "agent2"}

	r.startAgent(agentKey1, "agent1")
	r.startAgent(agentKey2, "agent2")

	st1, ok1 := r.startTool(key1, agentKey1, "inv1", "tool1")
	if !ok1 {
		t.Fatal("first startTool failed")
	}
	st2, ok2 := r.startTool(key2, agentKey2, "inv2", "tool2")
	if !ok2 {
		t.Fatal("second startTool failed with same SpanID but different TraceID")
	}

	// Both states should be independent.
	if st1 == st2 {
		t.Fatal("tool states should be independent")
	}

	// End one, the other should remain.
	r.endTool(key1)
	if r.getTool(key2) == nil {
		t.Fatal("second tool state should remain after ending first")
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := newRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := agentStateKey{
				InvocationID: "inv",
				Branch:       "root",
				AgentName:    "agent",
			}
			r.startAgent(key, "agent")
			r.incrementAgentInference(key)
			r.incrementAgentTool(key)
			r.endAgent(key)
		}(i)
	}
	wg.Wait()
}

func TestRegistryCleanupInvocationIdempotent(t *testing.T) {
	r := newRegistry()
	key := agentStateKey{InvocationID: "inv1", Branch: "root", AgentName: "agent1"}
	r.startAgent(key, "agent1")

	// Cleanup twice - should not panic.
	r.cleanupInvocation("inv1")
	r.cleanupInvocation("inv1")
}

func TestRegistryCleanupEmptyInvocation(t *testing.T) {
	r := newRegistry()
	// Should not panic on empty registry.
	r.cleanupInvocation("nonexistent")
}
