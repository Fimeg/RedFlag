-- Migration 027: Create scanner_config table for user-configurable scanner timeouts
-- Renumbered from 018 (F-B1-3: wrong file suffix, F-B1-13: duplicate number)
-- Fixed: removed GRANT to non-existent role (F-B1-4)
-- Fixed: added IF NOT EXISTS for idempotency (ETHOS #4)

CREATE TABLE IF NOT EXISTS scanner_config (
    scanner_name VARCHAR(50) PRIMARY KEY,
    timeout_ms BIGINT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CHECK (timeout_ms > 0 AND timeout_ms <= 7200000)
);

CREATE INDEX IF NOT EXISTS idx_scanner_config_updated_at ON scanner_config(updated_at);

-- Insert default timeout values for all scanners
INSERT INTO scanner_config (scanner_name, timeout_ms) VALUES
    ('system', 10000),
    ('storage', 10000),
    ('apt', 1800000),
    ('dnf', 1800000),
    ('docker', 60000),
    ('windows', 600000),
    ('winget', 120000),
    ('updates', 30000)
ON CONFLICT (scanner_name) DO NOTHING;
