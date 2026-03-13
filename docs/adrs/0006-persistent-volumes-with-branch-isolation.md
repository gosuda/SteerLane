# 0006: Persistent volumes with branch isolation

- Status: accepted
- Date: 2026-03-11

## Context

Agent runs need repository continuity across sessions while preventing concurrent tasks from mutating the same branch state.

## Decision

Maintain one persistent repository volume per project and create a session-scoped Git branch for each agent run.

## Consequences

- Repositories are cloned once and reused across agent sessions.
- Concurrent work is isolated by branch instead of duplicating full repositories.
- Git operations and cleanup become a first-class runtime concern.
