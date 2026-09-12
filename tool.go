package otelgenai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// ToolRequest describes a client-side tool execution.
type ToolRequest struct {
	Name string
	Type string
	ID   string
}

// ToolOperation represents an in-flight tool execution span.
type ToolOperation struct {
	span      trace.Span
	in        *Instrumenter
	ctx       context.Context
	startTime time.Time
	name      string
	mu        sync.Mutex
	finalized bool
}

// StartTool creates an INTERNAL span named "execute_tool {name}" with
// tool name/type and the enclosing agent name when available. It
// increments the nearest enclosing agent's tool-call counter.
func (in *Instrumenter) StartTool(ctx context.Context, req ToolRequest) (context.Context, *ToolOperation) {
	if in.cfg.disabled {
		return ctx, nil
	}
	spanName := semconv.OperationExecuteTool
	if req.Name != "" {
		spanName = fmt.Sprintf("%s %s", semconv.OperationExecuteTool, req.Name)
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, semconv.OperationExecuteTool),
	}
	if req.Name != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIToolName, req.Name))
	}
	toolType := req.Type
	if toolType == "" {
		toolType = semconv.ToolTypeFunction
	}
	attrs = append(attrs, attribute.String(semconv.AttrGenAIToolType, toolType))
	if req.ID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIToolCallID, req.ID))
	}

	// Add enclosing agent name if available.
	if agentObs := agentObserverFromContext(ctx); agentObs != nil {
		agentObs.addTool()
	}

	ctx, span := in.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)

	return ctx, &ToolOperation{
		span:      span,
		in:        in,
		ctx:       ctx,
		startTime: time.Now(),
		name:      req.Name,
	}
}

// End finalizes the tool execution span and emits the tool duration
// metric. It is idempotent. Failed tool calls are still counted as
// calls per the convention.
func (t *ToolOperation) End(err error) {
	if t == nil || t.span == nil {
		return
	}
	t.mu.Lock()
	if t.finalized {
		t.mu.Unlock()
		return
	}
	t.finalized = true
	t.mu.Unlock()

	if err != nil {
		et := t.in.classifyError(err)
		if et == ErrorTypeNone {
			et = ErrorTypeUnknown
		}
		t.span.SetAttributes(attribute.String(semconv.AttrErrorType, string(et)))
		t.span.SetStatus(codes.Error, "")
	} else {
		t.span.SetStatus(codes.Ok, "")
	}

	duration := time.Since(t.startTime).Seconds()
	t.span.End()

	var attrs []attribute.KeyValue
	if t.name != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIToolName, t.name))
	}
	t.in.metrics.executeToolDuration.Record(t.ctx, duration,
		metric.WithAttributes(attrs...),
	)
}

// TraceTool is a generic convenience function that wraps a tool
// execution with instrumentation. It starts a tool span, invokes the
// function, and ends the span with the result. It does not force a
// framework-specific tool interface.
func TraceTool[T any](in *Instrumenter, ctx context.Context, req ToolRequest, fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, op := in.StartTool(ctx, req)
	result, err := fn(ctx)
	op.End(err)
	return result, err
}
