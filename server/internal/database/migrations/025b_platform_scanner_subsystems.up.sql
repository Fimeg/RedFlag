-- Migration: 025_platform_scanner_subsystems
-- Purpose: Add platform-specific package scanner subsystems (apt, dnf, winget, windows)
-- Version: 0.1.29
-- Date: 2025-12-23

-- Add platform-specific subsystems for existing Linux agents
INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run, created_at, updated_at)
SELECT a.id, 'apt', true, 30, true, NOW(), NOW()
FROM agents a
WHERE LOWER(a.os_type) LIKE '%linux%'
  AND NOT EXISTS (SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = a.id AND subsystem = 'apt');

INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run, created_at, updated_at)
SELECT a.id, 'dnf', true, 240, true, NOW(), NOW()
FROM agents a
WHERE LOWER(a.os_type) LIKE '%linux%'
  AND NOT EXISTS (SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = a.id AND subsystem = 'dnf');

-- Add platform-specific subsystems for existing Windows agents
INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run, created_at, updated_at)
SELECT a.id, 'windows', true, 480, true, NOW(), NOW()
FROM agents a
WHERE LOWER(a.os_type) LIKE '%windows%'
  AND NOT EXISTS (SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = a.id AND subsystem = 'windows');

INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run, created_at, updated_at)
SELECT a.id, 'winget', true, 360, true, NOW(), NOW()
FROM agents a
WHERE LOWER(a.os_type) LIKE '%windows%'
  AND NOT EXISTS (SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = a.id AND subsystem = 'winget');

-- Trigger and function removed: the registration handler (agents.go)
-- creates subsystems during agent registration. A trigger is redundant
-- and causes "current transaction aborted" errors on fresh installs
-- when the handler's inserts collide with the trigger's inserts.
