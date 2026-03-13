-- name: CreateTenant :one
INSERT INTO tenants (name, slug, settings)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: UpdateTenant :one
UPDATE tenants SET name = $2, slug = $3, settings = $4, updated_at = now()
WHERE id = $1
RETURNING *;
