-- Migration 061: Device classification on agents (SERVER-001).
-- device_type is the agent's auto-detected form factor (DEVICE-001);
-- device_type_manual is the operator override (SERVER-002) — never written by
-- agent reports. Effective type = COALESCE(device_type_manual, device_type).
-- Existing agents default to 'server' (matches current behavior: every agent
-- so far is a server/desktop-class box treated identically).

ALTER TABLE agents ADD COLUMN IF NOT EXISTS device_type VARCHAR(20) NOT NULL DEFAULT 'server'
    CHECK (device_type IN ('server', 'desktop', 'phone', 'tablet'));
ALTER TABLE agents ADD COLUMN IF NOT EXISTS device_type_manual VARCHAR(20)
    CHECK (device_type_manual IN ('server', 'desktop', 'phone', 'tablet'));
ALTER TABLE agents ADD COLUMN IF NOT EXISTS device_model VARCHAR(255);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS os_distro VARCHAR(50);
