-- 000006_messenger_audit.up.sql
-- Messenger connections and audit log.

-- Per-tenant messenger platform configuration (encrypted at app level).
CREATE TABLE messenger_connections (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    platform   TEXT        NOT NULL
                           CHECK (platform IN ('slack', 'discord', 'telegram')),
    config     JSONB       NOT NULL DEFAULT '{}',
    channel_id TEXT,
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_messenger_connections_tenant_platform UNIQUE (tenant_id, platform)
);

-- Immutable audit log for compliance and debugging.
CREATE TABLE audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_type  TEXT        NOT NULL
                            CHECK (actor_type IN ('user', 'agent', 'system')),
    actor_id    TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    resource    TEXT        NOT NULL,
    resource_id TEXT        NOT NULL,
    details     JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Paginated reads: WHERE tenant_id = $1 ORDER BY created_at DESC.
CREATE INDEX idx_audit_log_tenant_created
    ON audit_log (tenant_id, created_at DESC);

-- Per-resource queries: WHERE tenant_id = $1 AND resource = $2 AND resource_id = $3.
CREATE INDEX idx_audit_log_tenant_resource
    ON audit_log (tenant_id, resource, resource_id);
