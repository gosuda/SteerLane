# SteerLane

## One board. Humans and agents. No context lost.

Kanban-driven agent orchestration for developers.

Run, steer, and review AI agent work from a single board.
Simple Mode for one-tap task resolution.
Steer Mode for remote agent control from your phone.

Supports Claude Code · Codex · Gemini CLI · OpenCode
Self-host or use Steerlane Cloud.

---

## Overview

Steerlane runs agent work through a Go control plane, persists task and session state in PostgreSQL and Redis, and serves an embedded Svelte dashboard from the same server process.

The repo currently supports:

- A Kanban board for task flow across `Backlog`, `In Progress`, `Review`, and `Done`
- Live agent-session monitoring with websocket updates, status changes, event logs, and token usage
- Human-in-the-loop question routing and resume flows for Slack, Discord, and Telegram
- Architectural Decision Records (ADRs) attached to project work
- Self-hosted and SaaS/cloud-capable deployment modes through `STEERLANE_MODE`

## Modes

- **Simple Mode** keeps task resolution lightweight: create or review work from the board, let the orchestrator dispatch the right agent runtime, and only step in when the system asks for input.
- **Steer Mode** is for remote control: answer HITL prompts, resume sessions, and monitor progress through connected messengers, including from your phone.

## How It Works

1. A task is created from the board or a connected messenger flow.
2. Steerlane selects a registered runtime such as Claude Code, Codex, Gemini CLI, or OpenCode.
3. The orchestrator runs the session, records events, and updates the dashboard in real time.
4. If an agent needs an answer, the HITL router pushes the question into the active conversation thread.
5. Your reply resumes execution and the session monitor keeps the full activity trail visible.
6. ADR records and task state stay attached to the work as it moves through the board.

## Quick Start

```bash
git clone https://github.com/gosuda/steerlane.git
cd steerlane
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8080` to access the dashboard.

For first-run self-hosted setup, optionally set these before starting the stack:

```bash
STEERLANE_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
STEERLANE_BOOTSTRAP_ADMIN_PASSWORD=changeme
STEERLANE_BOOTSTRAP_ADMIN_NAME=Admin
```

Core runtime defaults in `.env.example`:

- `STEERLANE_MODE=selfhosted`
- `STEERLANE_HTTP_ADDR=:8080`
- `STEERLANE_PUBLIC_BASE_URL=http://localhost:8080`
- `STEERLANE_POSTGRES_DSN=postgres://steerlane:steerlane@localhost:5432/steerlane?sslmode=disable`
- `STEERLANE_REDIS_ADDR=localhost:6379`

## Dashboard Surface

The embedded web app currently exposes:

- **Board views** for project task orchestration and drag-and-drop status changes
- **Session monitor views** for elapsed time, token accounting, live events, and HITL answers
- **ADR views** for browsing architectural decisions associated with project work
- **Auth and setup flows** for login, registration, messenger linking, and project settings

## Integrations

### Agent runtimes

- Claude Code
- Codex
- Gemini CLI
- OpenCode

### Messaging and control

- Slack
- Discord
- Telegram

### Platform dependencies

- Go 1.26+
- PostgreSQL
- Redis
- Docker / Docker Compose

## Local Development

Useful repo commands:

```bash
make build
make dev
make test
make test-race
make lint
make dashboard
```

`make dashboard` rebuilds the Svelte dashboard assets that the Go server embeds and serves.

## Architecture

- **Server entrypoint**: `cmd/steerlane`
- **Config**: environment-driven via `internal/config`
- **Runtime wiring**: agent registry, docker-backed CLI runtime, git operations, HITL router, notifier dispatch
- **Storage**: PostgreSQL for durable state and Redis for pub/sub
- **UI delivery**: Svelte dashboard built under `web/` and embedded into the server binary

## License

Steerlane is licensed under Elastic License 2.0. See [LICENSE](LICENSE) for the full terms and managed-service restrictions.
