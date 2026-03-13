# AGENTS.md — SteerLane

Use this file for SteerLane-specific contracts only. If a fact is one search away, leave it out.

## Generated Boundaries

- Rule: Treat `internal/store/postgres/sqlc/` and `web/src/lib/api/schema.d.ts` as generated artifacts.
  Why: `db/sqlc.yaml` generates the sqlc store layer from `db/queries/` plus `migrations/`, and the web client types are generated from `../openapi.json`; hand edits will drift from their sources.

- Rule: When API shapes change, regenerate in this order: start the server or otherwise expose `/openapi.json`, refresh `openapi.json`, then run `npm --prefix web run api:generate`.
  Why: the web generator reads `../openapi.json`; if that file is stale, the frontend silently compiles against the wrong contract.

## Boot And Runtime

- Rule: Assume database migrations run at server startup and a dirty migration state is fatal.
  Why: `cmd/steerlane/main.go` calls the embedded migration runner before opening the app, and `internal/migrate/migrate.go` aborts on dirty state instead of trying to recover.

- Rule: In self-hosted mode, preserve the implicit default tenant contract unless the corresponding web/auth flow is updated in the same change.
  Why: bootstrap creates tenant slug `default`, and the web session helpers fall back to `default` when no tenant is stored.

- Rule: If any bootstrap admin env var is set, all of email, password, and name must be set together.
  Why: partial bootstrap config is treated as invalid and fails bootstrap instead of creating a partial admin.

## Multi-Tenant And Auth Contracts

- Rule: Keep `tenant_id` scoping on every tenant-owned repository query and mutation, even for self-hosted paths.
  Why: ADR 0004 makes row-level tenant isolation a hard architecture invariant; self-hosted mode reuses the same storage model rather than bypassing it.

- Rule: Preserve the auth middleware before tenant middleware ordering on protected routes.
  Why: tenant context is derived from authenticated identity and the tenant middleware rejects requests when auth has not populated context first.

- Rule: Login and registration flows must resolve a tenant before user auth succeeds.
  Why: auth session handlers accept either `tenant_id` or `tenant_slug` and fail fast without one, so new auth entry points must keep that resolution step.

## Agent Session Isolation

- Rule: Keep agent execution concerns in the runtime/orchestration layer, not in domain types or messenger adapters.
  Why: ADR 0002 commits SteerLane to pluggable agent backends running in Docker-managed environments, with cleanup and resource control outside domain logic.

- Rule: Preserve one persistent repo volume per project and one `steerlane/<session-id>` branch per session.
  Why: ADR 0006 and the current `gitops`/`volume` code rely on reuse-by-volume plus branch isolation to avoid recloning repos and to keep concurrent agent runs from mutating the same branch.

## Messenger And HITL

- Rule: Route messenger integrations through the shared messenger abstraction and explicit link records, not platform-specific shortcuts in orchestration code.
  Why: ADRs 0005 and 0008 separate SteerLane auth from messenger identity mapping; HITL and notifications depend on linked messenger identities without making messenger platforms the source of truth for auth.

## Embedded Dashboard

- Rule: Keep backend reserved paths out of SPA fallback and keep embedded asset expectations aligned with the frontend build output.
  Why: the server serves `fs.Sub(web.Build, "build")` as a catch-all SPA, but `/api`, `/ws`, messenger webhooks, `/healthz`, `/openapi.json`, and `/docs` must remain backend-owned paths.

## Go 1.26+ Guardrails

- Rule: For new HTTP routes, prefer stdlib `http.NewServeMux()` patterns like `"GET /path"` and keep method/path dispatch in the mux instead of manual method switching.
  Why: the server already standardizes on method-aware `ServeMux` registrations across API, webhook, WebSocket, and auth routes, so mixing router styles makes transport behavior harder to reason about.

- Rule: In tests, prefer `t.Context()` when code under test accepts a context.
  Why: the test suite already uses `t.Context()` heavily, which keeps cancellation tied to the test lifecycle and avoids stray background work.

- Rule: When iterating a fixed count, prefer `for i := range n` over a C-style counter loop unless you need custom step logic.
  Why: the codebase already uses integer-range loops where they fit, and Go 1.26 makes that form available without compatibility tradeoffs.
