# 0001: Select Huma v2 with stdlib ServeMux

- Status: accepted
- Date: 2026-03-11

## Context

SteerLane needs a Go HTTP API that produces OpenAPI 3.1 docs, validates requests consistently, and stays close to the standard library for portability.

## Decision

Use Huma v2 for API schema generation and request/response handling, layered on top of Go's `net/http` and `http.ServeMux`.

## Consequences

- We keep standard-library routing and middleware composition.
- We get generated OpenAPI and typed request contracts without introducing a full framework.
- Transport code stays replaceable while the API surface remains well documented.
