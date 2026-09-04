CREATE TABLE IF NOT EXISTS agent_inventory (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id            UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    inventory_ecosystem VARCHAR(50) NOT NULL,
    item_name           TEXT NOT NULL,
    item_version        TEXT NOT NULL,
    description         TEXT,
    arch                VARCHAR(50),
    install_time        TIMESTAMPTZ,
    size_bytes          BIGINT,
    vendor              TEXT,
    metadata            JSONB DEFAULT '{}',
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (agent_id, inventory_ecosystem, item_name)
);

CREATE INDEX IF NOT EXISTS idx_agent_inventory_agent
    ON agent_inventory (agent_id);

CREATE INDEX IF NOT EXISTS idx_agent_inventory_ecosystem
    ON agent_inventory (inventory_ecosystem);

CREATE INDEX IF NOT EXISTS idx_agent_inventory_agent_ecosystem
    ON agent_inventory (agent_id, inventory_ecosystem);

CREATE INDEX IF NOT EXISTS idx_agent_inventory_last_seen
    ON agent_inventory (last_seen_at);
