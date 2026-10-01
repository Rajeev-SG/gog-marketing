-- Dropping this column intentionally forfeits product ownership metadata.
-- Re-applying 004 requires re-importing or re-flagging product-managed accounts.
ALTER TABLE IF EXISTS google_connections DROP COLUMN IF EXISTS product_managed;
