-- Migration 030: Seed operational timeout settings (F-E1-3)
-- These values replace hardcoded constants in main.go and timeout.go
-- Category: 'operational' — runtime-configurable server behavior

INSERT INTO security_settings
    (id, category, key, value, value_type, description, requires_restart, validation_rules)
VALUES
    (gen_random_uuid(), 'operational', 'offline_check_interval_seconds', '120',
     'number', 'How often to check for offline agents (seconds)', false,
     '{"min": 30, "max": 3600}'),

    (gen_random_uuid(), 'operational', 'offline_threshold_minutes', '10',
     'number', 'Minutes before an agent is marked offline', false,
     '{"min": 2, "max": 60}'),

    (gen_random_uuid(), 'operational', 'token_cleanup_interval_hours', '24',
     'number', 'Hours between expired token cleanup runs', false,
     '{"min": 1, "max": 168}'),

    (gen_random_uuid(), 'operational', 'sent_command_timeout_hours', '2',
     'number', 'Hours before sent commands are marked timed out', false,
     '{"min": 1, "max": 24}'),

    (gen_random_uuid(), 'operational', 'pending_command_timeout_minutes', '30',
     'number', 'Minutes before pending commands are marked timed out', false,
     '{"min": 5, "max": 120}'),

    (gen_random_uuid(), 'operational', 'timeout_check_interval_minutes', '5',
     'number', 'How often the timeout service checks for stuck commands (minutes)', false,
     '{"min": 1, "max": 30}')
ON CONFLICT (category, key) DO NOTHING;
