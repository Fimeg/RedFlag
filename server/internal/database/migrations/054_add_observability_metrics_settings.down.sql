DELETE FROM security_settings
WHERE category = 'observability'
  AND key IN ('metrics_enabled', 'metrics_token_hash');
