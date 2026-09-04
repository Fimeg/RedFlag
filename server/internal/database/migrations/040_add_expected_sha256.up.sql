-- Layer 1: Hash Registry
-- Add expected_sha256 column to current_package_state for install-time verification
ALTER TABLE current_package_state ADD COLUMN IF NOT EXISTS expected_sha256 VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_expected_sha256 ON current_package_state(expected_sha256);
