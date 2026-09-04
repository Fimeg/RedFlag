-- Migration 038: Policy switches and update-reconciler timeout
-- These rows give the server runtime-configurable policy without code edits.
-- Categories used:
--   policy      — feature gates (allow/require/skip behaviors)
--   operational — runtime tuning (intervals, thresholds)

INSERT INTO security_settings
    (id, category, key, value, value_type, description, requires_restart, validation_rules)
VALUES
    (gen_random_uuid(), 'policy', 'allow_dry_runs', 'true',
     'boolean', 'When false, dry_run_update commands are rejected before being queued', false,
     '{}'),

    (gen_random_uuid(), 'policy', 'require_nonce', 'true',
     'boolean', 'When false, agent update nonce validation is skipped (development only)', false,
     '{}'),

    (gen_random_uuid(), 'policy', 'auto_heartbeat_enabled', 'true',
     'boolean', 'When true, signAndCreateCommand auto-queues enable_heartbeat (system) before state-changing commands', false,
     '{}'),

    (gen_random_uuid(), 'operational', 'update_stuck_minutes', '5',
     'number', 'Minutes after which an agent with is_updating=true and no live update_agent command is reconciled to false', false,
     '{"min": 2, "max": 60}')
ON CONFLICT (category, key) DO NOTHING;
