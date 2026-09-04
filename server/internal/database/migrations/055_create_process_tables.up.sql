-- Process scan snapshots: one row per on-demand scan
CREATE TABLE IF NOT EXISTS agent_process_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    command_id TEXT,
    process_count INT NOT NULL DEFAULT 0,
    scanned_at TIMESTAMP NOT NULL,
    scan_duration_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_process_snapshots_agent_latest
    ON agent_process_snapshots(agent_id, created_at DESC);

-- Individual processes within a snapshot
CREATE TABLE IF NOT EXISTS agent_processes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id UUID NOT NULL REFERENCES agent_process_snapshots(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    pid INT NOT NULL,
    name TEXT NOT NULL,
    path TEXT,
    cmdline TEXT,
    cwd TEXT,
    state CHAR(1),
    uid INT,
    gid INT,
    euid INT,
    egid INT,
    "user" TEXT,
    "group" TEXT,
    tty INT,
    tty_name TEXT,
    cpu_seconds_user FLOAT DEFAULT 0,
    cpu_seconds_system FLOAT DEFAULT 0,
    cpu_percent FLOAT DEFAULT 0,
    rss_bytes BIGINT DEFAULT 0,
    vms_bytes BIGINT DEFAULT 0,
    mem_percent FLOAT DEFAULT 0,
    threads INT DEFAULT 0,
    nice INT DEFAULT 0,
    start_time_seconds BIGINT DEFAULT 0,
    parent_pid INT,
    process_group_id INT,
    elevation_status TEXT,
    on_disk INT DEFAULT -1,
    disk_bytes_read BIGINT DEFAULT 0,
    disk_bytes_written BIGINT DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processes_snapshot ON agent_processes(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_processes_agent ON agent_processes(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_processes_name ON agent_processes(name);
CREATE INDEX IF NOT EXISTS idx_processes_user ON agent_processes("user");
CREATE INDEX IF NOT EXISTS idx_processes_state ON agent_processes(state);
CREATE INDEX IF NOT EXISTS idx_processes_cpu ON agent_processes(cpu_percent DESC);

-- Related data (open files, sockets, pipes, env, memory map, namespaces, listening ports)
CREATE TABLE IF NOT EXISTS agent_process_related (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    process_id UUID NOT NULL REFERENCES agent_processes(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_process_related_process ON agent_process_related(process_id);
CREATE INDEX IF NOT EXISTS idx_process_related_type ON agent_process_related(relation_type);
