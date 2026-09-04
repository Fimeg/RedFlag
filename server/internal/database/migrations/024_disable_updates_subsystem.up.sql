-- Migration 024: Disable legacy updates subsystem
-- Purpose: Clean up from monolithic scan_updates to individual scanners
-- Fixed: removed self-insert into schema_migrations (F-B1-1)
-- Fixed: removed reference to non-existent deprecated column (F-B1-2)

-- Disable all 'updates' subsystems (legacy monolithic scanner)
-- Uses existing enabled/auto_run columns (no deprecated column needed)
UPDATE agent_subsystems
SET enabled = false,
    auto_run = false,
    updated_at = NOW()
WHERE subsystem = 'updates';
