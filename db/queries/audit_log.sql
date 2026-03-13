-- name: AppendAuditEntry :one
INSERT INTO audit_log (tenant_id, actor_type, actor_id, action, resource, resource_id, details)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAuditEntryByID :one
SELECT * FROM audit_log WHERE id = $1 AND tenant_id = $2;

-- name: ListAuditByTenant :many
SELECT * FROM audit_log WHERE tenant_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListAuditByTenantAfter :many
SELECT * FROM audit_log
WHERE tenant_id = $1
  AND (created_at < $2 OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: ListAuditByResource :many
SELECT * FROM audit_log WHERE tenant_id = $1 AND resource = $2 AND resource_id = $3
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: ListAuditByResourceAfter :many
SELECT * FROM audit_log
WHERE tenant_id = $1 AND resource = $2 AND resource_id = $3
  AND (created_at < $4 OR (created_at = $4 AND id < $5))
ORDER BY created_at DESC, id DESC
LIMIT $6;
