package otelgenai

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// InternalOperation represents an in-flight internal operation span that
// is neither an inference nor a tool call. It is intended for operations
// like MCP resources/read and prompts/get that should appear in the trace
// hierarchy without affecting agent counters.
//
// InternalOperation does not emit metrics. It creates an INTERNAL span
// with the supplied attributes, classifies errors via the configured
// ErrorClassifier, and is nil-safe and idempotent on End.
type InternalOperation struct {
	span      trace.Span
	in        *Instrumenter
	mu        sync.Mutex
	finalized bool
}

// StartInternalOperation creates an INTERNAL span with the given name and
// attributes. It does not increment any agent counters. When
// instrumentation is disabled, returns a nil operation whose End is
// nil-safe.
//
// When a conversation ID is present in ctx (via WithConversationID), it
// is attached to the span as gen_ai.conversation.id. The caller-supplied
// attrs slice is never mutated; a new slice is constructed when a
// conversation ID is present.
func (in *Instrumenter) StartInternalOperation(
	ctx context.Context,
	name string,
	attrs ...attribute.KeyValue,
) (context.Context, *InternalOperation) {
	if in.cfg.disabled {
		return ctx, nil
	}
	spanAttrs := attrs
	if convID, ok := ConversationIDFromContext(ctx); ok && convID != "" {
		spanAttrs = make([]attribute.KeyValue, 0, len(attrs)+1)
		spanAttrs = append(spanAttrs, attrs...)
		spanAttrs = append(spanAttrs, attribute.String(semconv.AttrGenAIConversationID, convID))
	}
	ctx, span := in.tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(spanAttrs...),
	)
	return ctx, &InternalOperation{
		span: span,
		in:   in,
	}
}

// End finalizes the internal operation span. It is idempotent. Error
// classification uses the instrumenter's configured ErrorClassifier,
// matching ToolOperation and InferenceOperation behavior. No metric is
// emitted; internal operations are generic and don't have a natural
// metric instrument.
func (op *InternalOperation) End(err error) {
	if op == nil || op.span == nil {
		return
	}
	op.mu.Lock()
	if op.finalized {
		op.mu.Unlock()
		return
	}
	op.finalized = true
	op.mu.Unlock()

	if err != nil {
		et := op.in.ClassifyError(err)
		op.span.SetAttributes(attribute.String(semconv.AttrErrorType, string(et)))
		op.span.SetStatus(codes.Error, "")
	} else {
		op.span.SetStatus(codes.Ok, "")
	}
	op.span.End()
}
