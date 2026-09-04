-- Add missing command statuses to the check constraint
-- This allows 'timed_out', 'cancelled', and 'running' statuses that the application uses

-- First drop the existing constraint
ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_status_check;

-- Add the new constraint with all valid statuses
ALTER TABLE agent_commands
ADD CONSTRAINT agent_commands_status_check
CHECK (status::text = ANY (ARRAY[
    'pending'::character varying,
    'sent'::character varying,
    'running'::character varying,
    'completed'::character varying,
    'failed'::character varying,
    'timed_out'::character varying,
    'cancelled'::character varying
]::text[]));