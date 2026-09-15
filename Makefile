GOPROXY ?= https://proxy.golang.org,direct
GOWORK ?= on

.PHONY: all build test test-race vet fmt tidy lint vuln bench downstream clean

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

clean:
	rm -f coverage.out benchmarks.txt
