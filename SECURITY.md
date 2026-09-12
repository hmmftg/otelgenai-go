# Security Policy

## Supported versions

While the major version is `0`, only the latest released minor of each
module receives security fixes. The core, OpenAI adapter, and Anthropic
adapter are versioned and released independently.

## Reporting a vulnerability

Please report suspected vulnerabilities privately. Do **not** open a
public issue. Email the maintainers with a description, reproduction
steps if available, and the affected module and version.

We will acknowledge receipt within a reasonable window, coordinate a
fix, and credit reporters in the release notes unless they prefer to
remain anonymous.

## What this library considers sensitive

`otelgenai-go` is a telemetry library for generative AI applications.
By design it observes provider calls that frequently contain prompts,
model responses, tool arguments, tool results, and potentially
credentials or PII. The library's safety contract is:

- **No content is captured by default.** Spans and metrics contain only
  metadata such as operation, provider, model, server address, request
  parameters, response ID/model, finish reasons, and token usage.
- **No credentials, cookies, authorization headers, or raw HTTP bodies
  are read or recorded.**
- **No raw error messages are recorded.** Failures are classified to a
  low-cardinality `error.type` and span status is set without a
  description.
- **Opt-in content projection is explicit and user-controlled.** When
  a `ContentProjector` is configured, the user is responsible for the
  redaction policy of their own data. The library validates projected
  output against the convention schema and a byte limit, and drops the
  projection on any failure, panic, or oversize result with no raw
  fallback.
- **Provider SDK calls are never altered.** The library does not parse
  wire bodies, change retry settings, or modify request options,
  responses, stream events, or returned errors.

### Important note for users enabling content projection

When you supply a `ContentProjector`, projected span attributes may
contain prompts, responses, tool arguments, or tool results that your
projector chose to retain. You are responsible for ensuring your
projector redacts PII and secrets before they reach telemetry. Export
your spans and metrics only to destinations whose data-handling policy
is compatible with the content your projector emits.

## Out of scope

- Vulnerabilities in upstream OpenTelemetry, the OpenAI Go SDK, or the
  Anthropic Go SDK should be reported to those projects directly.
- Vulnerabilities that require the user to configure an unsafe
  projector that leaks their own data are not library defects, but
  please report them so documentation can be improved.
