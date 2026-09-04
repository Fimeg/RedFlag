-- Migration 061 down: remove device classification columns.

ALTER TABLE agents DROP COLUMN IF EXISTS os_distro;
ALTER TABLE agents DROP COLUMN IF EXISTS device_model;
ALTER TABLE agents DROP COLUMN IF EXISTS device_type_manual;
ALTER TABLE agents DROP COLUMN IF EXISTS device_type;
