-- Align update_version_history.update_status vocabulary with
-- current_package_state.status: 'updated' instead of 'success'.
-- The old constraint allowed ('success', 'failed', 'rollback').
-- The new constraint allows ('updated', 'failed', 'rollback').
-- No data migration is needed — the existing rows are timestamped
-- audit records, and the vocabulary change only affects future inserts.
ALTER TABLE update_version_history DROP CONSTRAINT IF EXISTS update_version_history_update_status_check;
ALTER TABLE update_version_history ADD CONSTRAINT update_version_history_update_status_check
  CHECK (update_status IN ('updated', 'failed', 'rollback'));
