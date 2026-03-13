-- 000005_agent_runtime.up.sql
-- Agent runtime: repo volumes, sessions, HITL questions, and deferred FKs.

-- Docker volume tracking per project (1:1).
CREATE TABLE repo_volumes (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    volume_name     TEXT        NOT NULL UNIQUE,
    last_fetched_at TIMESTAMPTZ,
    size_bytes      BIGINT      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent execution sessions tied to a task.
CREATE TABLE agent_sessions (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id    UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id       UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    agent_type    TEXT        NOT NULL
                              CHECK (agent_type IN ('claude', 'codex', 'gemini', 'opencode', 'acp')),
    status        TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'running', 'waiting_hitl', 'completed', 'failed', 'cancelled')),
    container_id  TEXT,
    branch_name   TEXT,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    error         TEXT,
    metadata      JSONB       NOT NULL DEFAULT '{}',
    retry_count   INT         NOT NULL DEFAULT 0,
    retry_at      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_sessions_tenant_id  ON agent_sessions (tenant_id);
CREATE INDEX idx_agent_sessions_project_id ON agent_sessions (project_id);
CREATE INDEX idx_agent_sessions_task_id    ON agent_sessions (task_id);
CREATE INDEX idx_agent_sessions_status     ON agent_sessions (status);

-- Human-in-the-loop questions routed through messenger platforms.
CREATE TABLE hitl_questions (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_session_id    UUID        NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    question            TEXT        NOT NULL,
    options             JSONB,
    messenger_thread_id TEXT,
    messenger_platform  TEXT,
    answer              TEXT,
    answered_by         UUID        REFERENCES users(id) ON DELETE SET NULL,
    status              TEXT        NOT NULL DEFAULT 'pending'
                                    CHECK (status IN ('pending', 'answered', 'timeout', 'cancelled')),
    timeout_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at         TIMESTAMPTZ
);

CREATE INDEX idx_hitl_questions_tenant_id        ON hitl_questions (tenant_id);
CREATE INDEX idx_hitl_questions_agent_session_id  ON hitl_questions (agent_session_id);
CREATE INDEX idx_hitl_questions_status            ON hitl_questions (status);

-- Deferred foreign keys: adrs and tasks reference agent_sessions.
-- These columns were created as bare UUID in migration 000004 to break the cycle.
ALTER TABLE adrs
    ADD CONSTRAINT fk_adrs_agent_session
    FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE SET NULL;

ALTER TABLE tasks
    ADD CONSTRAINT fk_tasks_agent_session
    FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE SET NULL;
