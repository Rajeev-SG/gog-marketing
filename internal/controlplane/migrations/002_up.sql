ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS last_error_category text NOT NULL DEFAULT '';
