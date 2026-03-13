ALTER TABLE hitl_questions
    ADD COLUMN timeout_notification_claimed_at TIMESTAMPTZ,
    ADD COLUMN timeout_notified_at TIMESTAMPTZ;

CREATE INDEX idx_hitl_questions_timeout_notification_claimed_at
    ON hitl_questions (timeout_notification_claimed_at)
    WHERE status = 'timeout';

CREATE INDEX idx_hitl_questions_timeout_notified_at
    ON hitl_questions (timeout_notified_at)
    WHERE status = 'timeout';
