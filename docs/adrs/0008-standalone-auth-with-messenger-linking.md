# 0008: Standalone auth with messenger linking

- Status: accepted
- Date: 2026-03-11

## Context

SteerLane needs direct web authentication plus trusted mapping between product users and messenger identities for HITL and notifications.

## Decision

Use SteerLane-managed authentication for API and dashboard access, and maintain explicit user-to-messenger link records for platform identity mapping.

## Consequences

- Web access and messenger routing remain decoupled but consistent.
- Slack account linking becomes an integration concern instead of the primary auth model.
- Notification delivery can target linked users without exposing messenger-specific logic throughout the system.
