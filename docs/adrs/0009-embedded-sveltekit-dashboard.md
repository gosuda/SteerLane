# 0009: Embedded SvelteKit dashboard

- Status: accepted
- Date: 2026-03-11

## Context

The platform needs a modern dashboard for board state, ADR browsing, and live agent monitoring, while self-hosted deployments benefit from a single binary distribution model.

## Decision

Build the frontend in SvelteKit and embed the generated static assets into the Go server binary for runtime serving.

## Consequences

- The dashboard can be shipped with the backend as one deployable artifact.
- Local frontend development remains independent from the production serving model.
- Build output must stay in sync with backend static asset serving.
