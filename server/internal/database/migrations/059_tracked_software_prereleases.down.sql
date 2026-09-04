-- Reverse of 059: drop the prerelease-tracking opt-in column.

ALTER TABLE tracked_software
    DROP COLUMN IF EXISTS track_prereleases;
