ALTER TABLE google_connections ADD COLUMN IF NOT EXISTS product_managed boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS google_connections_product_subject_idx
  ON google_connections (organization_id, google_subject)
  WHERE product_managed AND google_subject <> '' AND status <> 'disconnected';

CREATE UNIQUE INDEX IF NOT EXISTS google_connections_product_email_idx
  ON google_connections (organization_id, lower(google_email))
  WHERE product_managed AND google_email <> '' AND status <> 'disconnected';
