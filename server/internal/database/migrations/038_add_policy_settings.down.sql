-- Reverse of 038_add_policy_settings.up.sql
DELETE FROM security_settings WHERE category = 'policy' AND key IN (
    'allow_dry_runs',
    'require_nonce',
    'auto_heartbeat_enabled'
);
DELETE FROM security_settings WHERE category = 'operational' AND key = 'update_stuck_minutes';
