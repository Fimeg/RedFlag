-- DOCKER-ENRICHED-SCAN: Container, stack, and engine version tracking.
-- Extends the agent's scan_docker report with container-level data,
-- compose stack grouping, and Docker engine version.

-- 1. Docker containers reported by agents
CREATE TABLE IF NOT EXISTS docker_containers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL,
    container_id    VARCHAR(32) NOT NULL,   -- short Docker container ID (12 chars)
    name            TEXT NOT NULL,
    image           TEXT NOT NULL,
    image_id        TEXT NOT NULL,
    state           VARCHAR(32) NOT NULL,   -- running, stopped, paused, etc.
    health          VARCHAR(32) NOT NULL DEFAULT '', -- healthy, unhealthy, starting, ''
    stack_name      TEXT NOT NULL DEFAULT '',
    ports           TEXT NOT NULL DEFAULT '',
    created_at_epoch BIGINT NOT NULL DEFAULT 0,
    labels          JSONB NOT NULL DEFAULT '{}',
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, container_id)
);

CREATE INDEX IF NOT EXISTS idx_docker_containers_agent
    ON docker_containers (agent_id);

CREATE INDEX IF NOT EXISTS idx_docker_containers_stack
    ON docker_containers (agent_id, stack_name)
    WHERE stack_name <> '';

CREATE INDEX IF NOT EXISTS idx_docker_containers_state
    ON docker_containers (agent_id, state);

-- 2. Docker Compose stacks derived from container labels
CREATE TABLE IF NOT EXISTS docker_stacks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL,
    name            TEXT NOT NULL,
    container_count INT NOT NULL DEFAULT 0,
    running_count   INT NOT NULL DEFAULT 0,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, name)
);

CREATE INDEX IF NOT EXISTS idx_docker_stacks_agent
    ON docker_stacks (agent_id);

-- 3. Engine version on the agents table (if not already present)
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS docker_version TEXT NOT NULL DEFAULT '';

-- 4. Add used flag and size to docker_images if not present
ALTER TABLE docker_images
    ADD COLUMN IF NOT EXISTS used BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE docker_images
    ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;

ALTER TABLE docker_images
    ADD COLUMN IF NOT EXISTS image_created_at TIMESTAMPTZ;
