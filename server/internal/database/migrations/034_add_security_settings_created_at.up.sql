-- Migration 034: Add created_at and created_by to security_settings
-- The table was created in migration 020 with updated_at/updated_by but not
-- the create-side columns. Query code (security_settings.go) selects and
-- inserts created_at / created_by, causing "failed to initialize default
-- security settings" at startup. Doctrine: AUDIT_TASKS.md §4.

ALTER TABLE security_settings
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id);
