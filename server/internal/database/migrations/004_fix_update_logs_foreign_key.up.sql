-- Fix foreign key relationship for update_logs table to reference current_package_state instead of update_packages
-- This ensures compatibility with the new event sourcing system

-- First, drop the existing foreign key constraint
ALTER TABLE update_logs DROP CONSTRAINT IF EXISTS update_logs_update_package_id_fkey;

-- Add the new foreign key constraint to reference current_package_state
ALTER TABLE update_logs
ADD CONSTRAINT update_logs_update_package_id_fkey
FOREIGN KEY (update_package_id) REFERENCES current_package_state(id) ON DELETE SET NULL;

-- Add index for better performance on the new foreign key
CREATE INDEX IF NOT EXISTS idx_logs_update_package ON update_logs(update_package_id);