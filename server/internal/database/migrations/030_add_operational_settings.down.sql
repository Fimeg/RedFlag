-- Migration 030 rollback: Remove operational timeout settings
DELETE FROM security_settings WHERE category = 'operational';
