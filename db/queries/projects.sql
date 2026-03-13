-- name: CreateProject :one
INSERT INTO projects (tenant_id, name, repo_url, branch, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1 AND tenant_id = $2;

-- name: LockProjectForUpdate :one
SELECT * FROM projects WHERE id = $1 AND tenant_id = $2 FOR UPDATE;

-- name: ListProjectsByTenant :many
SELECT * FROM projects WHERE tenant_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListProjectsByTenantAfter :many
SELECT * FROM projects
WHERE tenant_id = $1
  AND (created_at < $2 OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: UpdateProject :one
UPDATE projects SET name = $2, repo_url = $3, branch = $4, settings = $5
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = $1 AND tenant_id = $2;
