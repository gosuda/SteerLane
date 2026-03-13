# 0003: Separate ADR timeline from kanban tasks

- Status: accepted
- Date: 2026-03-11

## Context

Task boards explain what work exists, but they do not preserve architectural intent or decision history well enough for long-running agent workflows.

## Decision

Model ADRs as a project-scoped append-only timeline above the kanban task layer, with monotonic sequence numbers independent of task state.

## Consequences

- Architectural context remains readable even when tasks are split, retried, or closed.
- Tasks can reference ADRs without ADRs being reduced to board cards.
- We must maintain separate repository, API, and UI flows for ADR history.
