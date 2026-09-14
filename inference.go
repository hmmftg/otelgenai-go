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

	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
	"github.com/hmmftg/otelgenai-go/pricing"
)

// Operation is the canonical GenAI operation name.
type Operation string

// Request is the provider-neutral description of a GenAI inference
// request. Only fields relevant to the operation need to be set.
type Request struct {
	Operation      Operation
	Provider       string
	Model          string
	ServerAddress  string
	ServerPort     int
	MaxTokens      int64
	Temperature    float64
	HasTemperature bool
	TopP           float64
	HasTopP        bool
	Seed           int64
	HasSeed        bool
	StopSequences  []string
	Streaming      bool
	// Content fields are only populated when a projector is configured.
	SystemInstructions string
	InputMessages      []Message
	ToolDefinitions    []ToolDefinition
}

// Response is the provider-neutral result of a GenAI inference
// operation.
type Response struct {
	Model          string
	ID             string
	FinishReasons  []string
	Usage          Usage
	OutputMessages []Message
}

// Usage holds provider-reported token usage. The library does not
// infer missing usage values.
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
}

// InferenceOperation represents an in-flight inference span. Call
// [InferenceOperation.End] exactly once to finalize.
type InferenceOperation struct {
	span           trace.Span
	in             *Instrumenter
	ctx            context.Context
	startTime      time.Time
	operation      Operation
	model          string
	provider       string
	streaming      bool
	mu             sync.Mutex
	finalized      bool
	firstChunkTime time.Time
	hasFirstChunk  bool
}

// StartInference creates a CLIENT span named "{operation} {model}" and
// returns an InferenceOperation. Required request attributes are set
// at span creation so samplers can use them. If required metadata is
// absent, a no-op operation is returned.
func (in *Instrumenter) StartInference(ctx context.Context, req Request) (context.Context, *InferenceOperation) {
	if in.cfg.disabled || string(req.Operation) == "" || req.Model == "" {
		return ctx, in.nopInference()
	}

	attrs := []attribute.KeyValue{
		attribute.String(semconv.AttrGenAIOperationName, string(req.Operation)),
		attribute.String(semconv.AttrGenAIRequestModel, req.Model),
		attribute.String(semconv.AttrGenAISystem, req.Provider),
	}
	if req.ServerAddress != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIServerAddress, req.ServerAddress))
	}
	if req.ServerPort > 0 {
		attrs = append(attrs, attribute.Int(semconv.AttrGenAIServerPort, req.ServerPort))
	}
	if req.MaxTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIRequestMaxTokens, req.MaxTokens))
	}
	if req.HasTemperature {
		attrs = append(attrs, attribute.Float64(semconv.AttrGenAIRequestTemperature, req.Temperature))
	}
	if req.HasTopP {
		attrs = append(attrs, attribute.Float64(semconv.AttrGenAIRequestTopP, req.TopP))
	}
	if req.HasSeed {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIRequestSeed, req.Seed))
	}
	if len(req.StopSequences) > 0 {
		attrs = append(attrs, attribute.StringSlice(semconv.AttrGenAIRequestStopSequences, req.StopSequences))
	}
	if req.Streaming {
		attrs = append(attrs, attribute.Bool(semconv.AttrGenAIRequestStreaming, true))
	}

	// Opt-in content projection at start.
	if in.hasProjector() && req.SystemInstructions != "" {
		if proj := in.project(ContentKindSystemInstructions, ContentValue{
			Kind:               ContentKindSystemInstructions,
			SystemInstructions: req.SystemInstructions,
		}); proj != "" {
			attrs = append(attrs, attribute.String(semconv.AttrGenAISystemInstructions, proj))
		}
	}
	if in.hasProjector() && len(req.InputMessages) > 0 {
		if proj := in.project(ContentKindInputMessages, ContentValue{
			Kind:     ContentKindInputMessages,
			Messages: req.InputMessages,
		}); proj != "" {
			attrs = append(attrs, attribute.String(semconv.AttrGenAIInputMessages, proj))
		}
	}
	if in.hasProjector() && len(req.ToolDefinitions) > 0 {
		if proj := in.project(ContentKindToolDefinitions, ContentValue{
			Kind:            ContentKindToolDefinitions,
			ToolDefinitions: req.ToolDefinitions,
		}); proj != "" {
			attrs = append(attrs, attribute.String(semconv.AttrGenAIToolDefinitions, proj))
		}
	}

	spanName := fmtSpanName(string(req.Operation), req.Model)
	ctx, span := in.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)

	// Attach agent observer to context so inference calls are counted.
	agentObs := agentObserverFromContext(ctx)
	op := &InferenceOperation{
		span:      span,
		in:        in,
		ctx:       ctx,
		startTime: time.Now(),
		operation: req.Operation,
		model:     req.Model,
		provider:  req.Provider,
		streaming: req.Streaming,
	}
	ctx = contextWithInferenceOp(ctx, op)
	if agentObs != nil {
		agentObs.addInference()
	}

	return ctx, op
}

// End finalizes the inference operation. It is idempotent under
// concurrent or repeated calls. It records response metadata, usage,
// and error classification without recording raw errors.
func (op *InferenceOperation) End(resp Response, err error) {
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

	var attrs []attribute.KeyValue
	if resp.Model != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIResponseModel, resp.Model))
	}
	if resp.ID != "" {
		attrs = append(attrs, attribute.String(semconv.AttrGenAIResponseID, resp.ID))
	}
	if len(resp.FinishReasons) > 0 {
		attrs = append(attrs, attribute.StringSlice(semconv.AttrGenAIResponseFinishReasons, resp.FinishReasons))
	}

	// Record usage.
	if resp.Usage.InputTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageInputTokens, resp.Usage.InputTokens))
	}
	if resp.Usage.OutputTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageOutputTokens, resp.Usage.OutputTokens))
	}
	if resp.Usage.CacheReadTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageCacheReadInputTokens, resp.Usage.CacheReadTokens))
	}
	if resp.Usage.CacheWriteTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageCacheWriteInputTokens, resp.Usage.CacheWriteTokens))
	}
	if resp.Usage.ReasoningTokens > 0 {
		attrs = append(attrs, attribute.Int64(semconv.AttrGenAIUsageReasoningTokens, resp.Usage.ReasoningTokens))
	}

	// Estimate cost if a pricing resolver is configured.
	if op.in.pricing != nil {
		key := pricing.ModelPricingKey{System: op.provider, Model: op.model}
		type resolveResult struct {
			price pricing.Price
			ok    bool
		}
		result, perr := safety.GuardedCallValue(func() (resolveResult, error) {
			p, ok := op.in.pricing.Resolve(key)
			return resolveResult{price: p, ok: ok}, nil
		})
		if perr != nil {
			safety.GuardedDiagnostic(op.in.diag, safety.Diagnostic{
				Stage: "pricing", Reason: safety.ReasonResolverPanic,
			})
		} else if result.ok {
			usage, valid := pricing.Normalize(
				resp.Usage.InputTokens, resp.Usage.OutputTokens,
				resp.Usage.CacheReadTokens, resp.Usage.CacheWriteTokens,
			)
			if valid && pricing.ValidatePrice(result.price) {
				cost := pricing.Estimate(usage, result.price)
				if pricing.ValidCost(cost) && cost > 0 {
					attrs = append(attrs, attribute.Float64("gen_ai.usage.estimated_cost", cost))
				}
			} // else: inconsistent usage or invalid price, omit cost
		} // else: unknown model, omit cost
	}

	// Opt-in output content projection.
	if op.in.hasProjector() && len(resp.OutputMessages) > 0 {
		if proj := op.in.project(ContentKindOutputMessages, ContentValue{
			Kind:     ContentKindOutputMessages,
			Messages: resp.OutputMessages,
		}); proj != "" {
			attrs = append(attrs, attribute.String(semconv.AttrGenAIOutputMessages, proj))
		}
	}

	// Error handling: low-cardinality error.type, no raw message.
	if err != nil {
		et := op.in.classifyError(err)
		if et == ErrorTypeNone {
			et = ErrorTypeUnknown
		}
		attrs = append(attrs, attribute.String(semconv.AttrErrorType, string(et)))
		op.span.SetStatus(codes.Error, "")
	} else {
		op.span.SetStatus(codes.Ok, "")
	}

	if len(attrs) > 0 {
		op.span.SetAttributes(attrs...)
	}

	// Emit duration metric.
	duration := time.Since(op.startTime).Seconds()
	op.in.metrics.clientOperationDuration.Record(op.ctx, duration,
		metric.WithAttributes(
			attribute.String(semconv.AttrGenAIOperationName, string(op.operation)),
			attribute.String(semconv.AttrGenAISystem, op.provider),
			attribute.String(semconv.AttrGenAIRequestModel, op.model),
		),
	)

	// Emit token usage metrics.
	if resp.Usage.InputTokens > 0 {
		op.in.metrics.clientTokenUsage.Record(op.ctx, resp.Usage.InputTokens,
			metric.WithAttributes(
				attribute.String(semconv.AttrGenAIOperationName, string(op.operation)),
				attribute.String(semconv.AttrGenAISystem, op.provider),
				attribute.String(semconv.AttrGenAIRequestModel, op.model),
				attribute.String(semconv.AttrGenAITokenType, semconv.TokenTypeInput),
			),
		)
	}
	if resp.Usage.OutputTokens > 0 {
		op.in.metrics.clientTokenUsage.Record(op.ctx, resp.Usage.OutputTokens,
			metric.WithAttributes(
				attribute.String(semconv.AttrGenAIOperationName, string(op.operation)),
				attribute.String(semconv.AttrGenAISystem, op.provider),
				attribute.String(semconv.AttrGenAIRequestModel, op.model),
				attribute.String(semconv.AttrGenAITokenType, semconv.TokenTypeOutput),
			),
		)
	}

	// Record first-chunk time on span if streaming.
	if op.streaming && op.hasFirstChunk {
		ttfb := op.firstChunkTime.Sub(op.startTime).Seconds()
		op.span.SetAttributes(attribute.Float64(semconv.AttrGenAIResponseTimeToFirstChunk, ttfb))
	}

	op.span.End()
}

// nopInference returns a no-op InferenceOperation used when required
// metadata is absent or instrumentation is disabled.
func (in *Instrumenter) nopInference() *InferenceOperation {
	return nil
}

// fmtSpanName formats an inference span name.
func fmtSpanName(operation, model string) string {
	return fmt.Sprintf("%s %s", operation, model)
}
