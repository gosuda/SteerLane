-- name: CreateAgentSession :one
INSERT INTO agent_sessions (tenant_id, project_id, task_id, agent_type, status, branch_name, metadata, retry_count, retry_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetAgentSessionByID :one
SELECT * FROM agent_sessions WHERE id = $1 AND tenant_id = $2;

-- name: ListAgentSessionsByProject :many
SELECT * FROM agent_sessions WHERE project_id = $1 AND tenant_id = $2
ORDER BY created_at DESC, id DESC;

-- name: ListAgentSessionsByTask :many
SELECT * FROM agent_sessions WHERE task_id = $1 AND tenant_id = $2
ORDER BY created_at DESC, id DESC;

-- name: ListAgentSessionsRetryReady :many
SELECT * FROM agent_sessions
WHERE retry_at IS NOT NULL AND retry_at <= $1
ORDER BY retry_at ASC, created_at ASC, id ASC
LIMIT $2;

-- name: UpdateAgentSessionStatus :one
UPDATE agent_sessions SET status = $2, container_id = $3, started_at = $4, completed_at = $5, error = $6
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: UpdateAgentSessionRetry :exec
UPDATE agent_sessions SET retry_count = $2, retry_at = $3
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id);
