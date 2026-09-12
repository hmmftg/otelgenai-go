# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for stable releases. While the major version is `0`, breaking changes may occur
in minor releases because the GenAI semantic conventions are still in
development.

## [Unreleased]

### Added

- Initial public release of the framework-neutral OpenTelemetry GenAI
  instrumentation core.
- OpenAI adapter covering Responses, Chat Completions, and Embeddings with
  buffered and streaming instrumentation.
- Anthropic adapter covering Messages with buffered and streaming
  instrumentation.
- Safe-by-default telemetry: no prompts, responses, credentials, cookies,
  or raw bodies are captured unless an explicit content projector is
  configured.
- One logical span per provider operation, enclosing all automatic SDK
  retries.
- Streaming instrumentation with time-to-first-chunk and per-output-chunk
  timing.
- Local `invoke_agent` and client-side `execute_tool` spans and metrics.
- Semantic-convention conformance and sensitive-data contract tests.

[Unreleased]: https://github.com/hmmftg/otelgenai-go/releases
