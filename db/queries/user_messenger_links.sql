-- name: CreateMessengerLink :one
INSERT INTO user_messenger_links (user_id, tenant_id, platform, external_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMessengerLink :one
SELECT * FROM user_messenger_links
WHERE tenant_id = $1 AND platform = $2 AND external_id = $3;

-- name: ListMessengerLinksByUser :many
SELECT * FROM user_messenger_links
WHERE user_id = $1 AND tenant_id = $2
ORDER BY created_at DESC, id DESC;

-- name: DeleteMessengerLinkByID :one
DELETE FROM user_messenger_links
WHERE id = $1 AND user_id = $2 AND tenant_id = $3
RETURNING *;
