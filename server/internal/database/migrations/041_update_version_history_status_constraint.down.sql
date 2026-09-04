-- Restore old constraint vocabulary: ('success', 'failed', 'rollback')
ALTER TABLE update_version_history DROP CONSTRAINT IF EXISTS update_version_history_update_status_check;
ALTER TABLE update_version_history ADD CONSTRAINT update_version_history_update_status_check
  CHECK (update_status IN ('success', 'failed', 'rollback'));
