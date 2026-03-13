# 0002: Adopt pluggable agent backends with Docker execution

- Status: accepted
- Date: 2026-03-11

## Context

SteerLane must orchestrate multiple coding agents over time without coupling the platform to a single vendor or CLI runtime.

## Decision

Define a pluggable agent backend interface and run agent sessions inside Docker-managed execution environments with persistent project volumes.

## Consequences

- Claude is the first backend, but other agent adapters can be added behind the same contract.
- Runtime isolation, resource controls, and per-session cleanup stay outside domain logic.
- Container orchestration adds operational complexity, but it prevents repository and process interference.
