-- Revert: rename 'installed' back to 'updated'.

UPDATE current_package_state SET status = 'updated' WHERE status = 'installed';

ALTER TABLE current_package_state
DROP CONSTRAINT IF EXISTS current_package_state_status_check;

ALTER TABLE current_package_state
ADD CONSTRAINT current_package_state_status_check
CHECK (status IN ('pending', 'approved', 'checking_dependencies', 'pending_dependencies',
                  'installing', 'updated', 'failed', 'ignored'));

UPDATE update_version_history SET update_status = 'updated' WHERE update_status = 'installed';

ALTER TABLE update_version_history
DROP CONSTRAINT IF EXISTS update_version_history_update_status_check;

ALTER TABLE update_version_history
ADD CONSTRAINT update_version_history_update_status_check
CHECK (update_status IN ('updated', 'failed', 'rollback'));
