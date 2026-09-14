package otelgenai_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/safety"
	"github.com/hmmftg/otelgenai-go/testutil"
)

// newAdapterTestInstrumenter creates an Instrumenter with in-memory metric
// and trace providers for adapter API tests. It returns the instrumenter
// and the recorder for metric/span inspection.
func newAdapterTestInstrumenter(t *testing.T, opts ...otelgenai.Option) (*otelgenai.Instrumenter, *testutil.Recorder) {
	t.Helper()
	rec := testutil.NewRecorder()
	defaultOpts := []otelgenai.Option{
		otelgenai.WithMeterProvider(rec.MeterProvider()),
		otelgenai.WithTracerProvider(rec.TracerProvider()),
	}
	instr, err := otelgenai.New(append(defaultOpts, opts...)...)
	if err != nil {
		t.Fatalf("otelgenai.New: %v", err)
	}
	t.Cleanup(func() { _ = rec.Shutdown(context.Background()) })
	return instr, rec
}

// findHistogramByName finds a histogram by name in the collected metrics.
// Returns the data point count.
func histogramDataPointCount(rm metricdata.ResourceMetrics, name string) int {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if h, ok := m.Data.(metricdata.Histogram[int64]); ok {
					return len(h.DataPoints)
				}
				if h, ok := m.Data.(metricdata.Histogram[float64]); ok {
					return len(h.DataPoints)
				}
			}
		}
	}
	return 0
}

func TestClassifyErrorNilReturnsNone(t *testing.T) {
	instr, _ := newAdapterTestInstrumenter(t)
	if got := instr.ClassifyError(nil); got != otelgenai.ErrorTypeNone {
		t.Fatalf("ClassifyError(nil) = %q, want %q", got, otelgenai.ErrorTypeNone)
	}
}

func TestClassifyErrorUsesConfiguredClassifier(t *testing.T) {
	custom := func(err error) otelgenai.ErrorType {
		return otelgenai.ErrorTypeTimeout
	}
	instr, _ := newAdapterTestInstrumenter(t, otelgenai.WithErrorClassifier(custom))
	if got := instr.ClassifyError(errors.New("boom")); got != otelgenai.ErrorTypeTimeout {
		t.Fatalf("ClassifyError = %q, want %q", got, otelgenai.ErrorTypeTimeout)
	}
}

func TestClassifyErrorIsolatesClassifierPanic(t *testing.T) {
	var diags atomic.Int32
	panicking := func(err error) otelgenai.ErrorType {
		panic("classifier boom")
	}
	instr, _ := newAdapterTestInstrumenter(t,
		otelgenai.WithErrorClassifier(panicking),
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonClassifierPanic {
				diags.Add(1)
			}
		}),
	)
	got := instr.ClassifyError(errors.New("boom"))
	if got != otelgenai.ErrorTypeUnknown {
		t.Fatalf("ClassifyError after panic = %q, want %q", got, otelgenai.ErrorTypeUnknown)
	}
	if diags.Load() != 1 {
		t.Fatalf("expected 1 classifier.panic diagnostic, got %d", diags.Load())
	}
}

func TestClassifyErrorDisabledReturnsUnknown(t *testing.T) {
	instr, _ := newAdapterTestInstrumenter(t, otelgenai.Disabled())
	if got := instr.ClassifyError(errors.New("boom")); got != otelgenai.ErrorTypeUnknown {
		t.Fatalf("disabled ClassifyError = %q, want %q", got, otelgenai.ErrorTypeUnknown)
	}
}

func TestReportInstrumentationFailureInvokesDiagnosticHandler(t *testing.T) {
	var mu sync.Mutex
	var seen []safety.Diagnostic
	instr, _ := newAdapterTestInstrumenter(t,
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			mu.Lock()
			seen = append(seen, d)
			mu.Unlock()
		}),
	)
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureModelStateConflict)
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidToolSpan)
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidMetricValue)
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureToolSystemResolverPanic)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 {
		t.Fatalf("expected 4 diagnostics, got %d", len(seen))
	}
}

func TestReportInstrumentationFailureIsolatedHandlerPanic(t *testing.T) {
	instr, _ := newAdapterTestInstrumenter(t,
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			panic("handler boom")
		}),
	)
	// Must not panic.
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureModelStateConflict)
}

func TestReportInstrumentationFailureDisabledIsNoop(t *testing.T) {
	var count atomic.Int32
	instr, _ := newAdapterTestInstrumenter(t,
		otelgenai.Disabled(),
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			count.Add(1)
		}),
	)
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureModelStateConflict)
	if count.Load() != 0 {
		t.Fatalf("disabled instrumenter should not invoke diagnostic handler, got %d", count.Load())
	}
}

func TestRecordInferenceUsageOnlyPositiveComponents(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordInferenceUsage(context.Background(), otelgenai.Usage{
		InputTokens:      100,
		OutputTokens:     200,
		CacheReadTokens:  50,
		CacheWriteTokens: 30,
		ReasoningTokens:  40,
	}, "test-system", "test-model", "chat")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.client.token.usage"); got != 2 {
		t.Fatalf("expected 2 data points (input+output), got %d", got)
	}
}

func TestRecordInferenceUsageOmitsZeroComponents(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordInferenceUsage(context.Background(), otelgenai.Usage{
		InputTokens:  0,
		OutputTokens: 0,
	}, "test-system", "test-model", "chat")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.client.token.usage"); got != 0 {
		t.Fatalf("expected 0 data points for zero usage, got %d", got)
	}
}

func TestRecordInferenceDurationRecordsValue(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordInferenceDuration(context.Background(), 500_000_000, "sys", "model", "chat")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.client.operation.duration"); got != 1 {
		t.Fatalf("expected 1 data point, got %d", got)
	}
}

func TestRecordInferenceDurationNegativeIsOmitted(t *testing.T) {
	var diags atomic.Int32
	instr, rec := newAdapterTestInstrumenter(t,
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonInvalidMetricValue {
				diags.Add(1)
			}
		}),
	)
	instr.RecordInferenceDuration(context.Background(), -1, "sys", "model", "chat")

	if diags.Load() != 1 {
		t.Fatalf("expected 1 invalid metric diagnostic, got %d", diags.Load())
	}
	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.client.operation.duration"); got != 0 {
		t.Fatalf("expected 0 data points for negative duration, got %d", got)
	}
}

func TestRecordAgentDurationRecordsValue(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordAgentDuration(context.Background(), 1_000_000_000, "agent1")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.invoke_agent.duration"); got != 1 {
		t.Fatalf("expected 1 data point, got %d", got)
	}
}

func TestRecordAgentInferenceCallsRecordsZero(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordAgentInferenceCalls(context.Background(), 0, "agent1")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.invoke_agent.inference_calls"); got != 1 {
		t.Fatalf("expected 1 data point for zero count, got %d", got)
	}
}

func TestRecordAgentToolCallsRecordsZero(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordAgentToolCalls(context.Background(), 0, "agent1")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.invoke_agent.tool_calls"); got != 1 {
		t.Fatalf("expected 1 data point for zero count, got %d", got)
	}
}

func TestRecordAgentInferenceCallsNegativeIsOmitted(t *testing.T) {
	var diags atomic.Int32
	instr, rec := newAdapterTestInstrumenter(t,
		otelgenai.WithDiagnosticHandler(func(d safety.Diagnostic) {
			if d.Reason == safety.ReasonInvalidMetricValue {
				diags.Add(1)
			}
		}),
	)
	instr.RecordAgentInferenceCalls(context.Background(), -1, "agent1")

	if diags.Load() != 1 {
		t.Fatalf("expected 1 invalid metric diagnostic, got %d", diags.Load())
	}
	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.invoke_agent.inference_calls"); got != 0 {
		t.Fatalf("expected 0 data points for negative count, got %d", got)
	}
}

func TestRecordToolDurationRecordsValue(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordToolDuration(context.Background(), 300_000_000, "tool1")

	rm := testutil.CollectMetrics(t, rec)
	if got := histogramDataPointCount(rm, "gen_ai.execute_tool.duration"); got != 1 {
		t.Fatalf("expected 1 data point, got %d", got)
	}
}

func TestRecordMethodsDisabledIsNoop(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t, otelgenai.Disabled())
	instr.RecordInferenceUsage(context.Background(), otelgenai.Usage{InputTokens: 100, OutputTokens: 200}, "sys", "model", "chat")
	instr.RecordInferenceDuration(context.Background(), 1, "sys", "model", "chat")
	instr.RecordAgentDuration(context.Background(), 1, "agent1")
	instr.RecordAgentInferenceCalls(context.Background(), 1, "agent1")
	instr.RecordAgentToolCalls(context.Background(), 1, "agent1")
	instr.RecordToolDuration(context.Background(), 1, "tool1")

	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		if len(sm.Metrics) > 0 {
			t.Fatalf("disabled instrumenter should produce no metrics, found %d", len(sm.Metrics))
		}
	}
}

func TestRecordMethodsConcurrentSafe(t *testing.T) {
	instr, _ := newAdapterTestInstrumenter(t)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			instr.RecordInferenceUsage(context.Background(), otelgenai.Usage{InputTokens: 10, OutputTokens: 20}, "sys", "model", "chat")
			instr.RecordInferenceDuration(context.Background(), 1, "sys", "model", "chat")
			instr.RecordAgentDuration(context.Background(), 1, "agent1")
			instr.RecordAgentInferenceCalls(context.Background(), 1, "agent1")
			instr.RecordAgentToolCalls(context.Background(), 1, "agent1")
			instr.RecordToolDuration(context.Background(), 1, "tool1")
		}()
	}
	wg.Wait()
}

// Verify that the adapter API uses the same instruments as core operations.
func TestAdapterAPIUsesSameInstrumentsAsCore(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)

	// Record via adapter API.
	instr.RecordAgentDuration(context.Background(), 1_000_000_000, "adapter-agent")
	instr.RecordAgentInferenceCalls(context.Background(), 2, "adapter-agent")
	instr.RecordAgentToolCalls(context.Background(), 3, "adapter-agent")

	// Record via core API.
	ctx := context.Background()
	ctx, agentOp := instr.StartAgent(ctx, otelgenai.AgentRequest{Name: "core-agent"})
	agentOp.End(nil)

	// Both should use the same instrument names.
	rm := testutil.CollectMetrics(t, rec)
	agentDur := histogramDataPointCount(rm, "gen_ai.invoke_agent.duration")
	if agentDur < 2 {
		t.Fatalf("expected at least 2 data points (adapter+core), got %d", agentDur)
	}
}

// Verify that the adapter API produces the same attribute dimensions as core.
func TestAdapterAPIInferenceUsageDimensions(t *testing.T) {
	instr, rec := newAdapterTestInstrumenter(t)
	instr.RecordInferenceUsage(context.Background(), otelgenai.Usage{
		InputTokens:  100,
		OutputTokens: 200,
	}, "test-sys", "test-model", "chat")

	rm := testutil.CollectMetrics(t, rec)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gen_ai.client.token.usage" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				t.Fatalf("expected Histogram[int64], got %T", m.Data)
			}
			for _, dp := range hist.DataPoints {
				if v, ok := dp.Attributes.Value(attribute.Key("gen_ai.system")); !ok || v.AsString() != "test-sys" {
					t.Errorf("expected gen_ai.system=test-sys, got %v", v)
				}
				if v, ok := dp.Attributes.Value(attribute.Key("gen_ai.request.model")); !ok || v.AsString() != "test-model" {
					t.Errorf("expected gen_ai.request.model=test-model, got %v", v)
				}
				if v, ok := dp.Attributes.Value(attribute.Key("gen_ai.operation.name")); !ok || v.AsString() != "chat" {
					t.Errorf("expected gen_ai.operation.name=chat, got %v", v)
				}
			}
		}
	}
}
