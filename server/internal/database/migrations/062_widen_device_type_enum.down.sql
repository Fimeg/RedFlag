-- Migration 062 down: restore the four-type enum. New classes fold to their
-- nearest legacy type so the narrower CHECK can attach.

UPDATE agents SET device_type = 'desktop' WHERE device_type = 'laptop';
UPDATE agents SET device_type = 'server' WHERE device_type IN ('vm', 'container');
UPDATE agents SET device_type_manual = 'desktop' WHERE device_type_manual = 'laptop';
UPDATE agents SET device_type_manual = 'server' WHERE device_type_manual IN ('vm', 'container');

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_device_type_check;
ALTER TABLE agents ADD CONSTRAINT agents_device_type_check
    CHECK (device_type IN ('server', 'desktop', 'phone', 'tablet'));

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_device_type_manual_check;
ALTER TABLE agents ADD CONSTRAINT agents_device_type_manual_check
    CHECK (device_type_manual IN ('server', 'desktop', 'phone', 'tablet'));
