-- name: CreateOAuthLink :one
INSERT INTO user_oauth_links (user_id, provider, provider_id, access_token, refresh_token)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOAuthLinkByProvider :one
SELECT * FROM user_oauth_links WHERE provider = $1 AND provider_id = $2;

-- name: ListOAuthLinksByUser :many
SELECT * FROM user_oauth_links WHERE user_id = $1;

-- name: DeleteOAuthLink :exec
DELETE FROM user_oauth_links WHERE id = $1;
