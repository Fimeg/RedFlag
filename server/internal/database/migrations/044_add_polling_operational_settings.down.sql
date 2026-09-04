-- Reverse of 044_add_polling_operational_settings.up.sql
DELETE FROM security_settings WHERE category = 'operational' AND key IN (
    'jitter_max_seconds',
    'backoff_base_seconds',
    'backoff_max_seconds'
);
