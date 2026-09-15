GOPROXY ?= https://proxy.golang.org,direct
GOWORK ?= on

.PHONY: all build test test-race vet fmt tidy lint vuln bench downstream clean no-replace-root no-replace-adapters verify-published-core verify-published-adapters

all: build test

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
	go fmt ./...

tidy:
	go mod tidy
	cd instrumentation/openai && go mod tidy
	cd instrumentation/anthropic && go mod tidy
	cd instrumentation/google-genai && go mod tidy
	cd instrumentation/mcp && go mod tidy
	cd instrumentation/adk && go mod tidy

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

vuln:
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed, skipping"

bench:
	go test -bench=. -benchmem ./... | tee benchmarks.txt

downstream:
	cd test/downstream/root-only && GOWORK=off go build ./...
	cd test/downstream/openai-only && GOWORK=off go build ./...
	cd test/downstream/anthropic-only && GOWORK=off go build ./...
	cd test/downstream/google-genai-only && GOWORK=off go build ./...
	cd test/downstream/mcp-only && GOWORK=off go build ./...
	cd test/downstream/adk-only && GOWORK=off go build ./...

# Sanitized no-replace validation: copies each module to a temp dir,
# strips local replaces, and resolves declared versions with GOWORK=off.
# Adapters fail until the core version they require is published.
no-replace-root:
	sh scripts/validate-no-replace.sh .

no-replace-adapters:
	sh scripts/validate-no-replace.sh instrumentation/openai
	sh scripts/validate-no-replace.sh instrumentation/anthropic
	sh scripts/validate-no-replace.sh instrumentation/google-genai
	sh scripts/validate-no-replace.sh instrumentation/mcp
	sh scripts/validate-no-replace.sh instrumentation/adk

# Post-publication external consumer verification: build unrelated
# modules that import each published tag through normal resolution.
verify-published-core:
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go v0.6.0 \
		github.com/hmmftg/otelgenai-go

verify-published-adapters:
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go/instrumentation/openai v0.6.0 \
		github.com/hmmftg/otelgenai-go/instrumentation/openai
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go/instrumentation/anthropic v0.6.0 \
		github.com/hmmftg/otelgenai-go/instrumentation/anthropic
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go/instrumentation/google-genai v0.6.0 \
		github.com/hmmftg/otelgenai-go/instrumentation/google-genai
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go/instrumentation/mcp v0.6.0 \
		github.com/hmmftg/otelgenai-go/instrumentation/mcp
	sh scripts/verify-published-consumer.sh \
		github.com/hmmftg/otelgenai-go/instrumentation/adk v0.6.0 \
		github.com/hmmftg/otelgenai-go/instrumentation/adk

clean:
	rm -f coverage.out benchmarks.txt
