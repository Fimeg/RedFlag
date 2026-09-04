-- Refresh-token rotation + reuse detection.
--
-- Each refresh token belongs to a family. A renewal mints a successor token in
-- the same family and marks the parent "consumed" (consumed_at + superseded_by).
-- Replaying a consumed token whose successor is ALSO consumed means the chain
-- advanced past it without this caller — i.e. a stolen/duplicated token is being
-- replayed. That is reuse: the whole family is revoked (fail-closed), forcing
-- re-registration. A consumed token whose successor is still UNCONSUMED is the
-- benign crash-before-save case (accept-previous-once grace).
--
-- See RAF/security/01-trust-boundaries.md for the full state machine.

ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS family_id UUID;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS superseded_by UUID;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ;

-- Backfill: every pre-rotation token is the root of its own family.
UPDATE refresh_tokens SET family_id = id WHERE family_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
