ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS tool_grants_json text NOT NULL DEFAULT '{}';

-- Start connections with an explicit empty grant map; migration 006 rewrites
-- any legacy wildcard map values to the curated read-only selectors.
UPDATE google_connections SET tool_grants_json = '{}' WHERE tool_grants_json IS NULL;
