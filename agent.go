package otelgenai

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// AgentRequest describes a local agent invocation.
type AgentRequest struct {
	Name        string
	Description string
	Model       string
}

// AgentOperation represents an in-flight agent invocation span.
type AgentOperation struct {
	span           trace.Span
	in             *Instrumenter
	ctx            context.Context
	startTime      time.Time
	name           string
	inferenceCalls int64
	toolCalls      int64
	mu             sync.Mutex
	finalized      bool
}

// agentObserver is stored in context to count inference and tool calls
// against the nearest enclosing agent.
type agentObserver struct {
	addInference func()
	addTool      func()
}

type agentCtxKey struct{}

func contextWithAgentObserver(ctx context.Context, obs *agentObserver) context.Context {
	return context.WithValue(ctx, agentCtxKey{}, obs)
}

func agentObserverFromContext(ctx context.Context) *agentObserver {
	v, _ := ctx.Value(agentCtxKey{}).(*agentObserver)
	return v
}

// WithoutAgentObserver returns a context derived from ctx with the
// local agent observer removed. It removes only the agent-observer
// state stored by StartAgent; all other context values, cancellation,
// deadlines, and trace context are preserved unchanged.
//
// This is intended for cross-boundary instrumentation scenarios (such
// as MCP server middleware) where the remote trace context should be
// carried but the local agent's tool/inference counters must not be
// incremented by the remote side. The agent observer is a private
// core context value, so callers cannot strip it without this helper.
func WithoutAgentObserver(ctx context.Context) context.Context {
	if agentObserverFromContext(ctx) == nil {
		return ctx
	}
	// Create a child context that shadows the agent observer key with
	// nil, effectively removing it for downstream lookups.
	return context.WithValue(ctx, agentCtxKey{}, (*agentObserver)(nil))
}

type inferenceOpCtxKey struct{}

func contextWithInferenceOp(ctx context.Context, op *InferenceOperation) context.Context {
	return context.WithValue(ctx, inferenceOpCtxKey{}, op)
}

// StartAgent creates an INTERNAL span named "invoke_agent {name}" (or
// just "invoke_agent" when no name exists). It stores the agent
// observer in context so nested inference and tool calls are counted
// against this agent, not ancestors.
func (in *Instrumenter) StartAgent(ctx context.Context, req AgentRequest) (context.Context, *AgentOperation) {
	if in.cfg.disabled {
		return ctx, nil
	}
	spanName := semconv.SpanNameAgentDefault
	if req.Name != "" {
		spanName = fmt.Sprintf("%s %s", semconv.OperationInvokeAgent, req.Name)
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, semconv.OperationInvokeAgent),
	}
	if req.Name != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIAgentName, req.Name))
	}
	if req.Description != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIAgentDescription, req.Description))
	}
	if req.Model != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIRequestModel, req.Model))
	}
	if convID := ConversationIDFromContext(ctx); convID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIConversationID, convID))
	}

	ctx, span := in.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)

	op := &AgentOperation{
		span:      span,
		in:        in,
		ctx:       ctx,
		startTime: time.Now(),
		name:      req.Name,
	}

	obs := &agentObserver{
		addInference: op.addInferenceCall,
		addTool:      op.addToolCall,
	}
	ctx = contextWithAgentObserver(ctx, obs)

	return ctx, op
}

func (a *AgentOperation) addInferenceCall() {
	atomic.AddInt64(&a.inferenceCalls, 1)
}

func (a *AgentOperation) addToolCall() {
	atomic.AddInt64(&a.toolCalls, 1)
}

// End finalizes the agent invocation span and emits agent metrics.
// It is idempotent.
func (a *AgentOperation) End(err error) {
	if a == nil || a.span == nil {
		return
	}
	a.mu.Lock()
	if a.finalized {
		a.mu.Unlock()
		return
	}
	a.finalized = true
	a.mu.Unlock()

	if err != nil {
		et := a.in.ClassifyError(err)
		a.span.SetAttributes(attribute.String(semconv.AttrErrorType, string(et)))
		a.span.SetStatus(codes.Error, "")
	} else {
		a.span.SetStatus(codes.Ok, "")
	}

	duration := time.Since(a.startTime).Seconds()
	a.span.End()

	// Emit agent metrics.
	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIAgentName, a.name),
	}
	if a.name == "" {
		attrs = nil
	}

	a.in.metrics.invokeAgentDuration.Record(a.ctx, duration,
		metric.WithAttributes(attrs...),
	)
	a.in.metrics.invokeAgentInferenceCalls.Record(a.ctx, atomic.LoadInt64(&a.inferenceCalls),
		metric.WithAttributes(attrs...),
	)
	a.in.metrics.invokeAgentToolCalls.Record(a.ctx, atomic.LoadInt64(&a.toolCalls),
		metric.WithAttributes(attrs...),
	)
}
