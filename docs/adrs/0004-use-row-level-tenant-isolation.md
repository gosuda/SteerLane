# 0004: Use row-level tenant isolation

- Status: accepted
- Date: 2026-03-11

## Context

The platform supports both SaaS multi-tenancy and self-hosted single-tenant deployments from one codebase.

## Decision

Store tenant ownership explicitly on tenant-scoped records and require tenant-aware repository queries and mutations across the application.

## Consequences

- Multi-tenant safety is enforced in the data model and repository layer.
- Self-hosted mode can bootstrap a default tenant without changing the architecture.
- Query discipline is mandatory because every tenant-owned access path must remain scoped.
