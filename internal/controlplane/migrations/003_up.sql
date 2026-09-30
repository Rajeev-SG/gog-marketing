ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS discovery_status_json text NOT NULL DEFAULT '{}';
