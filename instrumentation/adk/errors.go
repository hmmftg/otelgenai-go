package adk

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/hmmftg/otelgenai-go"
	"github.com/hmmftg/otelgenai-go/internal/semconv"
)

// classifyAndSetError classifies an error using the instrumenter's
// configured classifier (panic-isolated) and sets the low-cardinality
// error.type attribute and status on the active span. On success, sets
// OK status. This is used only for tool spans that are still active.
func classifyAndSetError(span trace.Span, in *otelgenai.Instrumenter, err error) {
	if err != nil {
		et := in.ClassifyError(err)
		if et == otelgenai.ErrorTypeNone {
			et = otelgenai.ErrorTypeUnknown
		}
		span.SetAttributes(attribute.String(semconv.AttrErrorType, string(et)))
		span.SetStatus(codes.Error, "")
		return
	}
	span.SetStatus(codes.Ok, "")
}
