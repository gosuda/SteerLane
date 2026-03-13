-- name: CreateAgentSessionEvent :one
INSERT INTO agent_session_events (tenant_id, agent_session_id, event_type, payload, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAgentSessionEventByID :one
SELECT *
FROM agent_session_events
WHERE id = $1 AND tenant_id = $2 AND agent_session_id = $3;

-- name: ListAgentSessionEventsBySession :many
SELECT *
FROM agent_session_events
WHERE tenant_id = $1 AND agent_session_id = $2
ORDER BY created_at ASC, id ASC
LIMIT $3;

-- name: ListAgentSessionEventsBySessionAfterCursor :many
SELECT *
FROM agent_session_events AS e
WHERE e.tenant_id = $1
  AND e.agent_session_id = $2
  AND (
    e.created_at > (
      SELECT cursor.created_at
      FROM agent_session_events AS cursor
      WHERE cursor.id = $3 AND cursor.tenant_id = $1 AND cursor.agent_session_id = $2
    )
    OR (
      e.created_at = (
        SELECT cursor.created_at
        FROM agent_session_events AS cursor
        WHERE cursor.id = $3 AND cursor.tenant_id = $1 AND cursor.agent_session_id = $2
      )
      AND e.id > $3
    )
  )
ORDER BY e.created_at ASC, e.id ASC
LIMIT $4;
