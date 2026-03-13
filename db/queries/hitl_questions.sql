-- name: CreateHITLQuestion :one
INSERT INTO hitl_questions (id, tenant_id, agent_session_id, question, options, messenger_thread_id, messenger_platform, status, timeout_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: DeleteHITLQuestion :exec
DELETE FROM hitl_questions WHERE id = $1 AND tenant_id = $2;

-- name: CancelPendingHITLQuestionsBySession :exec
UPDATE hitl_questions
SET status = 'cancelled'
WHERE tenant_id = $1 AND agent_session_id = $2 AND status IN ('pending', 'escalated');

-- name: ClearHITLTimeoutNotificationClaim :exec
UPDATE hitl_questions
SET timeout_notification_claimed_at = NULL
WHERE id = $1 AND tenant_id = $2 AND status = 'timeout';

-- name: GetHITLQuestionByID :one
SELECT * FROM hitl_questions WHERE id = $1 AND tenant_id = $2;

-- name: GetPendingHITLQuestionByThread :many
SELECT * FROM hitl_questions
WHERE tenant_id = $1 AND messenger_platform = $2 AND messenger_thread_id = $3 AND status = 'pending'
ORDER BY created_at DESC, id DESC
LIMIT 2;

-- name: UpdateHITLQuestionMessengerThread :one
UPDATE hitl_questions
SET messenger_platform = $3, messenger_thread_id = $4
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: AnswerHITLQuestion :one
UPDATE hitl_questions SET answer = $2, answered_by = $3, status = 'answered', answered_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status IN ('pending', 'escalated')
RETURNING *;

-- name: ResetHITLQuestionAnswer :one
UPDATE hitl_questions SET answer = NULL, answered_by = NULL, status = 'pending', answered_at = NULL
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'answered'
RETURNING *;

-- name: ReopenTimedOutHITLQuestion :one
UPDATE hitl_questions
SET status = 'pending', timeout_notification_claimed_at = NULL, timeout_notified_at = NULL
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'timeout'
RETURNING *;

-- name: ListHITLQuestionsBySession :many
SELECT * FROM hitl_questions WHERE agent_session_id = $1 AND tenant_id = $2
ORDER BY created_at DESC, id DESC;

-- name: ListExpiredPendingHITLQuestionsBefore :many
SELECT q.* FROM hitl_questions AS q
JOIN agent_sessions AS s ON s.id = q.agent_session_id AND s.tenant_id = q.tenant_id
WHERE q.status = 'pending' AND q.timeout_at <= $1 AND s.status IN ('pending', 'running', 'waiting_hitl', 'cancelled')
ORDER BY q.timeout_at ASC, q.created_at ASC, q.id ASC
LIMIT $2;

-- name: ListTimedOutHITLQuestions :many
SELECT q.* FROM hitl_questions AS q
JOIN agent_sessions AS s ON s.id = q.agent_session_id AND s.tenant_id = q.tenant_id
WHERE q.status = 'timeout' AND s.status IN ('pending', 'running', 'waiting_hitl')
ORDER BY q.timeout_at ASC, q.created_at ASC, q.id ASC
LIMIT $1;

-- name: ListUnnotifiedTimedOutHITLQuestions :many
SELECT q.*
FROM hitl_questions AS q
JOIN agent_sessions AS s ON s.id = q.agent_session_id AND s.tenant_id = q.tenant_id
WHERE q.status = 'timeout'
  AND q.timeout_notified_at IS NULL
  AND s.status IN ('cancelled', 'completed', 'failed')
  AND (q.timeout_notification_claimed_at IS NULL OR q.timeout_notification_claimed_at < $1)
ORDER BY q.timeout_at ASC, q.created_at ASC, q.id ASC
LIMIT $2;

-- name: MarkTimedOutHITLQuestion :one
UPDATE hitl_questions SET status = 'timeout'
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'pending'
RETURNING *;

-- name: ClaimHITLTimeoutNotification :one
UPDATE hitl_questions
SET timeout_notification_claimed_at = now()
WHERE id = $1
  AND tenant_id = sqlc.arg(tenant_id)
  AND status = 'timeout'
  AND timeout_notified_at IS NULL
  AND (timeout_notification_claimed_at IS NULL OR timeout_notification_claimed_at < sqlc.arg(stale_before))
RETURNING *;

-- name: MarkHITLTimeoutNotificationSent :one
UPDATE hitl_questions
SET timeout_notification_claimed_at = NULL, timeout_notified_at = now()
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'timeout' AND timeout_notified_at IS NULL
RETURNING *;

-- name: EscalateHITLQuestion :one
UPDATE hitl_questions
SET status = 'escalated', timeout_at = sqlc.arg(new_timeout_at)
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'pending'
RETURNING *;

-- name: ListEscalatedExpiredHITLQuestionsBefore :many
SELECT q.* FROM hitl_questions AS q
JOIN agent_sessions AS s ON s.id = q.agent_session_id AND s.tenant_id = q.tenant_id
WHERE q.status = 'escalated' AND q.timeout_at <= $1 AND s.status IN ('pending', 'running', 'waiting_hitl')
ORDER BY q.timeout_at ASC, q.created_at ASC, q.id ASC
LIMIT $2;

-- name: MarkTimedOutEscalatedHITLQuestion :one
UPDATE hitl_questions SET status = 'timeout'
WHERE id = $1 AND tenant_id = sqlc.arg(tenant_id) AND status = 'escalated'
RETURNING *;

-- name: MarkTimedOutHITLQuestionsBefore :many
WITH expired AS (
    SELECT id
    FROM hitl_questions AS h
    WHERE h.status = 'pending' AND h.timeout_at <= $1
    ORDER BY h.timeout_at ASC, h.created_at ASC, h.id ASC
    LIMIT $2
)
UPDATE hitl_questions AS q
SET status = 'timeout'
FROM expired
WHERE q.id = expired.id
RETURNING q.*;
