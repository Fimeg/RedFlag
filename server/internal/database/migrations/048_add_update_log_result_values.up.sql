-- LIFECYCLE-002: Expand update_logs.result CHECK to include started and running.
-- The original constraint only allowed 'success', 'failed', 'partial'.
-- started: progress notification from the agent (action has begun)
-- running: reserved for future use (in-flight status sent as secondary reports)
-- Both were previously mapped to 'failed' by the server default fallthrough.

-- Data migration: none needed — started/running were never stored (they were
-- always caught and remapped to 'failed' by the handler's isValidResult gate).
-- Any existing 'failed' rows that were *really* a started/running report are
-- indistinguishable from real failures and stay as-is.

-- Step 1: Drop the old constraint on update_logs.result.
ALTER TABLE update_logs
DROP CONSTRAINT IF EXISTS update_logs_result_check;

-- Step 2: Recreate with expanded value set.
ALTER TABLE update_logs
ADD CONSTRAINT update_logs_result_check
CHECK (result IN ('success', 'failed', 'partial', 'started', 'running'));
