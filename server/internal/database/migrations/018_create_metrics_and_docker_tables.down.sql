-- Down Migration: Remove metrics and docker_images tables
-- Purpose: Rollback migration 018 - remove separate tables for metrics and docker images

-- Drop indexes first
DROP INDEX IF EXISTS idx_metrics_agent_id;
DROP INDEX IF EXISTS idx_metrics_package_type;
DROP INDEX IF EXISTS idx_metrics_created_at;
DROP INDEX IF EXISTS idx_metrics_severity;

DROP INDEX IF EXISTS idx_docker_images_agent_id;
DROP INDEX IF EXISTS idx_docker_images_package_type;
DROP INDEX IF EXISTS idx_docker_images_created_at;
DROP INDEX IF EXISTS idx_docker_images_severity;
DROP INDEX IF EXISTS idx_docker_images_has_updates;

-- Drop the clean function
DROP FUNCTION IF EXISTS clean_misclassified_data();

-- Drop the tables
DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS docker_images;