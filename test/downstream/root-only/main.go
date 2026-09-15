// This module verifies that a consumer depending only on the root
// otelgenai-go module can build and use the core API without pulling in
// any provider SDK.
package main

import (
	"context"
	"fmt"

	"github.com/hmmftg/otelgenai-go"
)

func main() {
	// An external consumer must be able to name the diagnostic types.
	var seen []otelgenai.Diagnostic
	handler := otelgenai.DiagnosticHandler(func(d otelgenai.Diagnostic) {
		seen = append(seen, d)
	})
	instr, err := otelgenai.New(otelgenai.WithDiagnosticHandler(handler))
	if err != nil {
		panic(err)
	}
	// An adapter-facing failure maps to a public DiagnosticReason.
	instr.ReportInstrumentationFailure(otelgenai.InstrumentationFailureInvalidMetricValue)
	if len(seen) != 1 || seen[0].Reason != otelgenai.DiagnosticReasonInvalidMetricValue {
		panic("diagnostic not delivered to external consumer")
	}
	_, op := instr.StartInference(context.Background(), otelgenai.Request{
		Operation: otelgenai.Operation("chat"),
		Provider:  "openai",
		Model:     "gpt-4o",
	})
	op.End(otelgenai.Response{Model: "gpt-4o"}, nil)
	fmt.Println("root-only consumer built successfully")
}
