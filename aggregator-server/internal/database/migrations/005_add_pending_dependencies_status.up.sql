-- Add pending_dependencies and checking_dependencies status to support dependency confirmation workflow
ALTER TABLE current_package_state
DROP CONSTRAINT IF EXISTS current_package_state_status_check;

ALTER TABLE current_package_state
ADD CONSTRAINT current_package_state_status_check
CHECK (status IN ('pending', 'approved', 'updated', 'failed', 'ignored', 'installing', 'pending_dependencies', 'checking_dependencies'));

-- Also update any legacy tables if they exist
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'updates') THEN
        ALTER TABLE updates
        DROP CONSTRAINT IF EXISTS updates_status_check,
        ADD CONSTRAINT updates_status_check
        CHECK (status IN ('pending', 'approved', 'scheduled', 'installing', 'installed', 'failed', 'ignored', 'pending_dependencies', 'checking_dependencies'));
    END IF;
END $$;