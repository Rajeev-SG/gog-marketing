-- Picker selections are bound to one tenant/account/Clerk session. Only a
-- random session token's SHA-256 digest is stored, never Google credentials.
CREATE TABLE IF NOT EXISTS hosted_picker_sessions (
  session_hash TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  clerk_session_id TEXT NOT NULL,
  google_subject TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  FOREIGN KEY (tenant_id, connection_id)
    REFERENCES hosted_google_connections (tenant_id, id) ON DELETE CASCADE
);
