-- name: CreateRepoVolume :one
INSERT INTO repo_volumes (project_id, volume_name)
VALUES ($1, $2)
RETURNING *;

-- name: GetRepoVolumeByProject :one
SELECT * FROM repo_volumes WHERE project_id = $1;

-- name: UpdateRepoVolumeLastFetched :exec
UPDATE repo_volumes SET last_fetched_at = now(), size_bytes = $2 WHERE project_id = $1;

-- name: DeleteRepoVolume :exec
DELETE FROM repo_volumes WHERE project_id = $1;
