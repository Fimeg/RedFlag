-- Migration 054: Observability settings for Prometheus metrics
-- Metrics scraping uses a dedicated bearer token. Store only the SHA-256 hash
-- in security_settings; plaintext is accepted only through REDFLAG_METRICS_TOKEN
-- for bootstrap.

INSERT INTO security_settings
    (id, category, key, value, value_type, description, requires_restart, validation_rules)
VALUES
    (gen_random_uuid(), 'observability', 'metrics_enabled', 'false',
     'boolean', 'Enable the authenticated Prometheus /metrics endpoint.', false,
     '{}'),

    (gen_random_uuid(), 'observability', 'metrics_token_hash', '""',
     'string', 'SHA-256 hex digest of the dedicated Prometheus metrics bearer token.', false,
     '{"pattern": "^[a-fA-F0-9]{64}$|^$"}')
ON CONFLICT (category, key) DO NOTHING;
