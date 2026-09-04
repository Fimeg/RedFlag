-- Supply Chain Gate — capability token persistence.
-- Stores each minted Ed25519 capability token together with the fully-resolved
-- dependency closure (per-artifact name/version/sha256/source) it authorizes.
-- The privileged executor verifies signature + hashes independently; this table
-- is the server-side audit/replay record and the delivery source for the agent.
CREATE TABLE IF NOT EXISTS capability_tokens (
    token_id      UUID PRIMARY KEY,
    update_id     UUID REFERENCES current_package_state(id) ON DELETE SET NULL,
    agent_id      UUID NOT NULL,
    key_id        VARCHAR(32) NOT NULL,
    package_type  VARCHAR(32) NOT NULL,
    operation     VARCHAR(16) NOT NULL,
    closure       JSONB NOT NULL,
    issued_at     BIGINT NOT NULL,
    not_before    BIGINT NOT NULL,
    expires_at    BIGINT NOT NULL,
    signature     TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at  TIMESTAMPTZ,
    consumed_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_capability_tokens_agent ON capability_tokens(agent_id);
CREATE INDEX IF NOT EXISTS idx_capability_tokens_update ON capability_tokens(update_id);
CREATE INDEX IF NOT EXISTS idx_capability_tokens_expires ON capability_tokens(expires_at);
