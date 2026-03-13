-- name: CreateUser :one
INSERT INTO users (tenant_id, email, password_hash, name, role, avatar_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND tenant_id = $2;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND tenant_id = $2;

-- name: UpdateUser :one
UPDATE users SET email = $2, password_hash = $3, name = $4, role = $5, avatar_url = $6, updated_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1 AND tenant_id = $2;

-- name: ListUsersByTenant :many
SELECT * FROM users WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListUsersByTenantAfterCursor :many
SELECT * FROM users AS u
WHERE u.tenant_id = $1
  AND created_at < (
    SELECT cursor_user.created_at
    FROM users AS cursor_user
    WHERE cursor_user.id = $2 AND cursor_user.tenant_id = $1
  )
ORDER BY u.created_at DESC
LIMIT $3;
