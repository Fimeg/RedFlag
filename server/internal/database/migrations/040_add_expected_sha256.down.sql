-- Rollback: Remove expected_sha256 column from current_package_state
ALTER TABLE current_package_state DROP COLUMN IF EXISTS expected_sha256;
