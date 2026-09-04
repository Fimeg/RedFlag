-- Migration 024 rollback: Re-enable updates subsystem
UPDATE agent_subsystems
SET enabled = true,
    auto_run = false,
    updated_at = NOW()
WHERE subsystem = 'updates';
