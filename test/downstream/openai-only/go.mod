module downstream-openai-only

go 1.27.0

require (
	github.com/hmmftg/otelgenai-go v0.6.0
	github.com/hmmftg/otelgenai-go/instrumentation/openai v0.6.0
	github.com/openai/openai-go v1.12.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/log v0.22.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
)

replace github.com/hmmftg/otelgenai-go => ../../..

replace github.com/hmmftg/otelgenai-go/instrumentation/openai => ../../../instrumentation/openai
