-- Migration 060 rollback: Recreate token_seats (originally from migration 036).
-- This table was never used by application code, so the rollback exists only
-- to satisfy the migration runner's down-path contract.
CREATE TABLE IF NOT EXISTS token_seats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id UUID NOT NULL REFERENCES registration_tokens(id) ON DELETE CASCADE,
    seat_number INT NOT NULL,
    used_by_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    used_at TIMESTAMPTZ,
    UNIQUE(token_id, seat_number)
);

CREATE INDEX IF NOT EXISTS idx_token_seats_token_id ON token_seats(token_id);
CREATE INDEX IF NOT EXISTS idx_token_seats_agent_id ON token_seats(used_by_agent_id);
