DROP INDEX IF EXISTS idx_hitl_questions_timeout_notification_claimed_at;
DROP INDEX IF EXISTS idx_hitl_questions_timeout_notified_at;

ALTER TABLE hitl_questions
    DROP COLUMN IF EXISTS timeout_notification_claimed_at,
    DROP COLUMN IF EXISTS timeout_notified_at;
