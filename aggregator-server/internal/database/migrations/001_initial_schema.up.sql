-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Agents table
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hostname VARCHAR(255) NOT NULL,
    os_type VARCHAR(50) NOT NULL CHECK (os_type IN ('windows', 'linux', 'macos')),
    os_version VARCHAR(100),
    os_architecture VARCHAR(20),
    agent_version VARCHAR(20) NOT NULL,
    last_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(20) DEFAULT 'online' CHECK (status IN ('online', 'offline', 'error')),
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_os_type ON agents(os_type);
CREATE INDEX idx_agents_last_seen ON agents(last_seen);

-- Agent specs
CREATE TABLE agent_specs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    cpu_model VARCHAR(255),
    cpu_cores INTEGER,
    memory_total_mb INTEGER,
    disk_total_gb INTEGER,
    disk_free_gb INTEGER,
    network_interfaces JSONB,
    docker_installed BOOLEAN DEFAULT false,
    docker_version VARCHAR(50),
    package_managers TEXT[],
    collected_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_agent_specs_agent_id ON agent_specs(agent_id);

-- Update packages
CREATE TABLE update_packages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,
    package_name VARCHAR(500) NOT NULL,
    package_description TEXT,
    current_version VARCHAR(100),
    available_version VARCHAR(100) NOT NULL,
    severity VARCHAR(20) CHECK (severity IN ('critical', 'important', 'moderate', 'low', 'none')),
    cve_list TEXT[],
    kb_id VARCHAR(50),
    repository_source VARCHAR(255),
    size_bytes BIGINT,
    status VARCHAR(30) DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'scheduled', 'installing', 'installed', 'failed', 'ignored')),
    discovered_at TIMESTAMP DEFAULT NOW(),
    approved_by VARCHAR(255),
    approved_at TIMESTAMP,
    scheduled_for TIMESTAMP,
    installed_at TIMESTAMP,
    error_message TEXT,
    metadata JSONB,
    UNIQUE(agent_id, package_type, package_name, available_version)
);

CREATE INDEX idx_updates_status ON update_packages(status);
CREATE INDEX idx_updates_agent ON update_packages(agent_id);
CREATE INDEX idx_updates_severity ON update_packages(severity);
CREATE INDEX idx_updates_package_type ON update_packages(package_type);
CREATE INDEX idx_updates_composite ON update_packages(status, severity, agent_id);

-- Update logs
CREATE TABLE update_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    update_package_id UUID REFERENCES update_packages(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    result VARCHAR(20) NOT NULL CHECK (result IN ('success', 'failed', 'partial')),
    stdout TEXT,
    stderr TEXT,
    exit_code INTEGER,
    duration_seconds INTEGER,
    executed_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_logs_agent ON update_logs(agent_id);
CREATE INDEX idx_logs_result ON update_logs(result);
CREATE INDEX idx_logs_executed_at ON update_logs(executed_at DESC);

-- Agent tags
CREATE TABLE agent_tags (
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    tag VARCHAR(100) NOT NULL,
    PRIMARY KEY (agent_id, tag)
);

CREATE INDEX idx_agent_tags_tag ON agent_tags(tag);

-- Users (for authentication)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user' CHECK (role IN ('admin', 'user', 'readonly')),
    created_at TIMESTAMP DEFAULT NOW(),
    last_login TIMESTAMP
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);

-- Commands queue (for agent orchestration)
CREATE TABLE agent_commands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    command_type VARCHAR(50) NOT NULL,
    params JSONB,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'completed', 'failed')),
    created_at TIMESTAMP DEFAULT NOW(),
    sent_at TIMESTAMP,
    completed_at TIMESTAMP,
    result JSONB
);

CREATE INDEX idx_commands_agent_status ON agent_commands(agent_id, status);
CREATE INDEX idx_commands_created_at ON agent_commands(created_at DESC);
