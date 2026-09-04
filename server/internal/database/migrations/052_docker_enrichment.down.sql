-- Reverse DOCKER-ENRICHED-SCAN
DROP TABLE IF EXISTS docker_stacks;
DROP TABLE IF EXISTS docker_containers;
ALTER TABLE agents DROP COLUMN IF EXISTS docker_version;
ALTER TABLE docker_images DROP COLUMN IF EXISTS used;
ALTER TABLE docker_images DROP COLUMN IF EXISTS size_bytes;
ALTER TABLE docker_images DROP COLUMN IF EXISTS image_created_at;
