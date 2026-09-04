-- LIFECYCLE-001: Rename terminal success state from 'updated' to 'installed'.
-- This matches the Go PackageStatus constant StatusInstalled and what the UI expects.

-- Step 1: Migrate existing rows.
UPDATE current_package_state SET status = 'installed' WHERE status = 'updated';

-- Step 2: Drop and recreate the CHECK constraint.
ALTER TABLE current_package_state
DROP CONSTRAINT IF EXISTS current_package_state_status_check;

ALTER TABLE current_package_state
ADD CONSTRAINT current_package_state_status_check
CHECK (status IN ('pending', 'approved', 'checking_dependencies', 'pending_dependencies',
                  'installing', 'installed', 'failed', 'ignored'));

-- Step 3: Migrate update_version_history rows.
UPDATE update_version_history SET update_status = 'installed' WHERE update_status = 'updated';

-- Step 4: Update the update_version_history constraint (added by migration 041).
ALTER TABLE update_version_history
DROP CONSTRAINT IF EXISTS update_version_history_update_status_check;

ALTER TABLE update_version_history
ADD CONSTRAINT update_version_history_update_status_check
CHECK (update_status IN ('installed', 'failed', 'rollback'));
