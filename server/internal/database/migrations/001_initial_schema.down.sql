-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS agent_commands;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS agent_tags;
DROP TABLE IF EXISTS update_logs;
DROP TABLE IF EXISTS update_packages;
DROP TABLE IF EXISTS agent_specs;
DROP TABLE IF EXISTS agents;

-- Drop extension
DROP EXTENSION IF EXISTS "uuid-ossp";
