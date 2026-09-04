-- Migration 062: widen the device_type enum (DEVICE-001 follow-up).
-- Detection grew three classes: laptop (SMBIOS chassis), vm (hypervisor DMI
-- vendor/product), container (container= in PID 1 environ or
-- /run/systemd/container). 061's inline column CHECKs carry the
-- Postgres-generated names <table>_<column>_check.

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_device_type_check;
ALTER TABLE agents ADD CONSTRAINT agents_device_type_check
    CHECK (device_type IN ('server', 'desktop', 'laptop', 'phone', 'tablet', 'vm', 'container'));

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_device_type_manual_check;
ALTER TABLE agents ADD CONSTRAINT agents_device_type_manual_check
    CHECK (device_type_manual IN ('server', 'desktop', 'laptop', 'phone', 'tablet', 'vm', 'container'));
