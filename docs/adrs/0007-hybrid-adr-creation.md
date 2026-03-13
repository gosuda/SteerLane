# 0007: Hybrid ADR creation

- Status: accepted
- Date: 2026-03-11

## Context

Some important architectural choices are explicit during agent execution, while others only become clear after reviewing transcripts and diffs.

## Decision

Support two ADR creation paths: explicit agent tool calls for structured decisions and post-processing extraction for implicit decisions, with deduplication across both paths.

## Consequences

- Agents can record architectural intent in real time.
- Post-processing can recover missing context instead of depending on perfect agent behavior.
- Deduplication and review workflows are necessary to keep the timeline coherent.
