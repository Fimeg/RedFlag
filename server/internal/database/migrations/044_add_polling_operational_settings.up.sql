-- Migration 044: Polling resilience tuning (operational)
-- Fleet-wide check-in jitter and reconnect backoff curve. Delivered to agents
-- over the authenticated GET /api/v1/agents/:id/config channel and merged into
-- local agent config (agent/internal/config PollingConfig). Same tier as the
-- existing operational.update_stuck_minutes row from migration 038 — runtime
-- tuning, not policy doctrine.
INSERT INTO security_settings
    (id, category, key, value, value_type, description, requires_restart, validation_rules)
VALUES
    (gen_random_uuid(), 'operational', 'jitter_max_seconds', '30',
     'number', 'Cap on proportional check-in jitter (seconds). Spreads fleet check-ins to avoid thundering herd.', false,
     '{"min": 0, "max": 300}'),

    (gen_random_uuid(), 'operational', 'backoff_base_seconds', '10',
     'number', 'Reconnect backoff floor (seconds) after a failed check-in.', false,
     '{"min": 1, "max": 300}'),

    (gen_random_uuid(), 'operational', 'backoff_max_seconds', '300',
     'number', 'Reconnect backoff ceiling (seconds). Caps the exponential backoff curve.', false,
     '{"min": 10, "max": 3600}')
ON CONFLICT (category, key) DO NOTHING;
