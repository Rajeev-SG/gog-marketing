ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS product_managed boolean NOT NULL DEFAULT false;
