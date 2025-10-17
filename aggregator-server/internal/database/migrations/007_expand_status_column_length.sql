-- Expand status column to accommodate longer status values
-- checking_dependencies (23 chars) and pending_dependencies (21 chars) exceed current 20 char limit

ALTER TABLE current_package_state
ALTER COLUMN status TYPE character varying(30);

-- Update check constraint to match new length
ALTER TABLE current_package_state
DROP CONSTRAINT IF EXISTS current_package_state_status_check;

ALTER TABLE current_package_state
ADD CONSTRAINT current_package_state_status_check
CHECK (status::text = ANY (ARRAY['pending'::character varying, 'approved'::character varying, 'updated'::character varying, 'failed'::character varying, 'ignored'::character varying, 'installing'::character varying, 'pending_dependencies'::character varying, 'checking_dependencies'::character varying]::text[]));