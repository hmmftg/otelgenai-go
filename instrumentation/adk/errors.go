package adk

// This file previously held classifyAndSetError, which mutated a raw
// trace.Span using internal semconv attribute keys. It has been replaced
// by the constrained adapter API Instrumenter.ApplySpanOutcome, which
// the afterTool callback calls directly with the callback context.
