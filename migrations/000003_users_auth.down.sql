-- 000003_users_auth.down.sql
-- Drop in reverse dependency order.

DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS user_messenger_links;
DROP TABLE IF EXISTS user_oauth_links;
DROP TABLE IF EXISTS users;
