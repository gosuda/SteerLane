-- 000005_agent_runtime.down.sql
-- Drop deferred FKs first, then tables in reverse dependency order.

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS fk_tasks_agent_session;
ALTER TABLE adrs  DROP CONSTRAINT IF EXISTS fk_adrs_agent_session;

DROP TABLE IF EXISTS hitl_questions;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS repo_volumes;
