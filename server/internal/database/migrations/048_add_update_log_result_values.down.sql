-- LIFECYCLE-002: Revert update_logs.result CHECK to the original three values.
-- This is a schema-only rollback. Any rows that landed with 'started' or
-- 'running' would fail the restored constraint; in practice none should exist
-- because the agent never sends 'running' and 'started' is only written from
-- 048 migration onward. Callers that read those rows will see them as
-- constraint-violating garbage if this is rolled back after writes have happened.

-- Step 1: Drop the expanded constraint.
ALTER TABLE update_logs
DROP CONSTRAINT IF EXISTS update_logs_result_check;

-- Step 2: Restore the original constraint.
ALTER TABLE update_logs
ADD CONSTRAINT update_logs_result_check
CHECK (result IN ('success', 'failed', 'partial'));
