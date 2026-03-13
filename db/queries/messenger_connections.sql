-- name: CreateMessengerConnection :one
INSERT INTO messenger_connections (tenant_id, platform, config, channel_id, active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMessengerConnection :one
SELECT * FROM messenger_connections WHERE tenant_id = $1 AND platform = $2;

-- name: GetMessengerConnectionByChannel :one
SELECT * FROM messenger_connections
WHERE platform = $1 AND channel_id = $2 AND active = true;

-- name: UpdateMessengerConnection :one
UPDATE messenger_connections SET config = $3, channel_id = $4, active = $5
WHERE tenant_id = $1 AND platform = $2
RETURNING *;

-- name: ListMessengerConnectionsByTenant :many
SELECT * FROM messenger_connections WHERE tenant_id = $1;
