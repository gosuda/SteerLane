-- name: CreateTask :one
INSERT INTO tasks (tenant_id, project_id, adr_id, title, description, status, priority, assigned_to)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks WHERE id = $1 AND tenant_id = $2;

-- name: ListTasksByProject :many
SELECT * FROM tasks
WHERE project_id = sqlc.arg(project_id)
  AND tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(filter_status)::text IS NULL OR status = sqlc.narg(filter_status)::text)
  AND (sqlc.narg(filter_priority)::int IS NULL OR priority = sqlc.narg(filter_priority)::int)
ORDER BY priority ASC, created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListTasksByProjectAfter :many
SELECT * FROM tasks
WHERE project_id = sqlc.arg(project_id)
  AND tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(filter_status)::text IS NULL OR status = sqlc.narg(filter_status)::text)
  AND (sqlc.narg(filter_priority)::int IS NULL OR priority = sqlc.narg(filter_priority)::int)
  AND (
    priority > sqlc.arg(cursor_priority)
    OR (priority = sqlc.arg(cursor_priority) AND created_at < sqlc.arg(cursor_created_at))
    OR (priority = sqlc.arg(cursor_priority) AND created_at = sqlc.arg(cursor_created_at) AND id < sqlc.arg(cursor_id))
  )
ORDER BY priority ASC, created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateTask :one
UPDATE tasks SET title = $2, description = $3, priority = $4, assigned_to = $5, adr_id = $6, updated_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: TransitionTask :one
UPDATE tasks SET status = $2, agent_session_id = $3, updated_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = $1 AND tenant_id = $2;
