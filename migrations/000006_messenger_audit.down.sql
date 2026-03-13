-- 000006_messenger_audit.down.sql
-- Drop in reverse dependency order.

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS messenger_connections;
