-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, user_id, name, key_hash, prefix, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys WHERE prefix = $1;

-- name: ListAPIKeysByUser :many
SELECT * FROM api_keys WHERE user_id = $1 AND tenant_id = $2
ORDER BY created_at DESC;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = $1 AND tenant_id = $2;
