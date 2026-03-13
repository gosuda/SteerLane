-- 000004_projects_adrs_tasks.up.sql
-- Projects, ADRs, and kanban tasks.

CREATE TABLE projects (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    repo_url   TEXT        NOT NULL,
    branch     TEXT        NOT NULL DEFAULT 'main',
    settings   JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_projects_tenant_name UNIQUE (tenant_id, name)
);

CREATE INDEX idx_projects_tenant_id ON projects (tenant_id);

-- Architecture Decision Records. Sequence is monotonically increasing per project.
CREATE TABLE adrs (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sequence         INT         NOT NULL,
    title            TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'draft'
                                 CHECK (status IN ('draft', 'proposed', 'accepted', 'rejected', 'deprecated')),
    context          TEXT        NOT NULL DEFAULT '',
    decision         TEXT        NOT NULL DEFAULT '',
    drivers          TEXT[]      NOT NULL DEFAULT '{}',
    options          JSONB       NOT NULL DEFAULT '[]',
    consequences     JSONB       NOT NULL DEFAULT '{"good":[],"bad":[],"neutral":[]}',
    created_by       UUID        REFERENCES users(id) ON DELETE SET NULL,
    agent_session_id UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_adrs_project_sequence UNIQUE (project_id, sequence)
);

CREATE INDEX idx_adrs_tenant_id  ON adrs (tenant_id);
CREATE INDEX idx_adrs_project_id ON adrs (project_id);
CREATE INDEX idx_adrs_status     ON adrs (status);

-- Kanban tasks: backlog -> in_progress -> review -> done.
CREATE TABLE tasks (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    adr_id           UUID        REFERENCES adrs(id) ON DELETE SET NULL,
    title            TEXT        NOT NULL,
    description      TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL DEFAULT 'backlog'
                                 CHECK (status IN ('backlog', 'in_progress', 'review', 'done')),
    priority         INT         NOT NULL DEFAULT 0,
    assigned_to      UUID        REFERENCES users(id) ON DELETE SET NULL,
    agent_session_id UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tasks_tenant_id  ON tasks (tenant_id);
CREATE INDEX idx_tasks_project_id ON tasks (project_id);
CREATE INDEX idx_tasks_status     ON tasks (status);
