# Contributing to otelgenai-go

Thank you for your interest in contributing. This library provides
framework-neutral OpenTelemetry GenAI instrumentation for Go, and the
bar for correctness is intentionally high because telemetry that leaks
prompts, responses, or secrets is worse than no telemetry.

## Before you start

- Open an issue describing the change you intend to make for anything
  beyond a small fix. This avoids duplicate work and design mismatches.
- Note that the GenAI semantic conventions are still in development, so
  the pinned convention snapshot in `internal/semconv` is the source of
  truth for v0.1. Convention upgrades are deliberate, reviewed changes.

## Development environment

- Go 1.27 or newer.
- `git`.
- Optional: `golangci-lint` v2 and `govulncheck` for local checks.

## Repository layout

- The root module is `github.com/hmmftg/otelgenai-go` and contains the
  framework-neutral core.
- `instrumentation/openai` and `instrumentation/anthropic` are
  independently versioned modules. Each depends only on the root module
  and its official provider SDK.
- `internal/semconv`, `internal/safety`, and `internal/conformancetest`
  are internal packages and are not part of the public API.

## Safety rules for all changes

- Never capture prompts, responses, tool arguments, tool results,
  credentials, cookies, headers, or raw bodies by default.
- Never record raw error messages in span status, descriptions, or
  attributes. Use low-cardinality `error.type` classifications.
- Never fall back to raw content when a projector fails or panics.
- Never alter provider SDK retry settings, request options, response
  types, stream events, or returned errors.
- Recover panics only from library-owned extension callbacks
  (projectors, classifiers, diagnostic handlers), never from provider
  SDK calls or user operations.
- Keep provider SDK types out of the root module's public API.

## Running checks

From the repository root:

```sh
go mod tidy
go vet ./...
go test -race ./...
go build ./...
```

Each module must also build and test independently of the workspace:

```sh
# Root only
GOWORK=off go test ./...

# OpenAI adapter only
cd instrumentation/openai
GOWORK=off go test ./...

# Anthropic adapter only
cd instrumentation/anthropic
GOWORK=off go test ./...
```

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
Example: `feat(openai): instrument streaming Responses usage`.

## Releases

Releases are manual and module-scoped. A maintainer triggers the
release workflow with an explicit module selection and version. The
workflow validates every module but tags only the selected one, so v0.x
adapters can move at their own cadence while the core stabilizes.

## Licensing

Contributions are accepted under the project's MIT license.
