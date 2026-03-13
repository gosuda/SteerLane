# 0005: Abstract messenger platforms

- Status: accepted
- Date: 2026-03-11

## Context

Human-in-the-loop routing and notifications must work across Slack first, then other messengers, without rewriting orchestrator behavior for each platform.

## Decision

Use a platform-neutral messenger abstraction for sending messages, creating threads, updating messages, and dispatching notifications, with Slack as the first concrete adapter.

## Consequences

- HITL routing, notifier logic, and command parsing remain reusable across platforms.
- Slack-specific payload construction stays in the Slack integration package.
- Additional adapters still require platform-specific UX work, but not orchestration changes.
