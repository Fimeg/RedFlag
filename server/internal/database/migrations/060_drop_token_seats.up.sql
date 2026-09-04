-- Migration 060: Drop the token_seats table.
-- Created by migration 036 but never referenced by any Go code outside test files.
-- Seat tracking lives on registration_tokens.seats_used (incremented by the
-- mark_registration_token_used stored procedure). token_seats is dead weight.
DROP TABLE IF EXISTS token_seats CASCADE;
