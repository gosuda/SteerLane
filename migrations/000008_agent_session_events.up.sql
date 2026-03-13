-- 000008_agent_session_events.up.sql
-- Canonical persisted agent session events for replay.

CREATE TABLE agent_session_events (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_session_id UUID        NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    event_type       TEXT        NOT NULL,
    payload          JSONB       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_session_events_session_created_at
    ON agent_session_events (agent_session_id, created_at ASC, id ASC);

CREATE INDEX idx_agent_session_events_tenant_id
    ON agent_session_events (tenant_id);
