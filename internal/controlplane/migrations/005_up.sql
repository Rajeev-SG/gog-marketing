ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS tool_grants_json text NOT NULL DEFAULT '{}';
