-- 000004_projects_adrs_tasks.down.sql
-- Drop in reverse dependency order.

DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS adrs;
DROP TABLE IF EXISTS projects;
