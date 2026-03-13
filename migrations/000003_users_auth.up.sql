-- 000003_users_auth.up.sql
-- Users, OAuth links, messenger links, and API keys.

-- Users: one row per tenant membership. Email nullable for OAuth-only accounts.
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email         TEXT,
    password_hash TEXT,
    name          TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'member'
                              CHECK (role IN ('admin', 'member')),
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email)
);

CREATE INDEX idx_users_tenant_id ON users (tenant_id);

-- OAuth provider links (Google, GitHub, Slack, Discord).
CREATE TABLE user_oauth_links (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      TEXT        NOT NULL
                              CHECK (provider IN ('google', 'github', 'slack', 'discord')),
    provider_id   TEXT        NOT NULL,
    access_token  TEXT,
    refresh_token TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_oauth_provider_id UNIQUE (provider, provider_id)
);

CREATE INDEX idx_user_oauth_links_user_id ON user_oauth_links (user_id);

-- Messenger platform identities for HITL routing.
CREATE TABLE user_messenger_links (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    platform    TEXT        NOT NULL
                            CHECK (platform IN ('slack', 'discord', 'telegram')),
    external_id TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_messenger_tenant_platform_ext UNIQUE (tenant_id, platform, external_id)
);

CREATE INDEX idx_user_messenger_links_user_id ON user_messenger_links (user_id);

-- API keys for programmatic access. Only the SHA-256 hash is stored.
CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL,
    prefix       TEXT        NOT NULL,
    scopes       TEXT[]      NOT NULL DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_tenant_id ON api_keys (tenant_id);
CREATE INDEX idx_api_keys_prefix    ON api_keys (prefix);
