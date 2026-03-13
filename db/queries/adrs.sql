-- name: CreateADRWithNextSequence :one
INSERT INTO adrs (tenant_id, project_id, sequence, title, status, context, decision, drivers, options, consequences, created_by, agent_session_id)
VALUES (
    $1, $2,
    (SELECT COALESCE(MAX(sequence), 0) + 1 FROM adrs WHERE project_id = $2),
    $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: GetADRByID :one
SELECT * FROM adrs WHERE id = $1 AND tenant_id = $2;

-- name: ListADRsByProject :many
SELECT * FROM adrs WHERE project_id = $1 AND tenant_id = $2
ORDER BY sequence ASC, id ASC
LIMIT $3;

-- name: ListADRsByProjectAfter :many
SELECT * FROM adrs
WHERE project_id = $1 AND tenant_id = $2
  AND (sequence > $3 OR (sequence = $3 AND id > $4))
ORDER BY sequence ASC, id ASC
LIMIT $5;

-- name: UpdateADRStatus :one
UPDATE adrs SET status = $2, updated_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;
