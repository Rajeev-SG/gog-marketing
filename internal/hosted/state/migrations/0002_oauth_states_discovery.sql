-- Hosted v1 OAuth connection state and discovery status.
--
-- hosted_oauth_states holds one-use state/PKCE/nonce material bound to the
-- authenticated tenant, the intended Google connection, and the Clerk session
-- that started the flow. The compound foreign key guarantees a state cannot
-- outlive (or be completed against) a deleted or foreign connection.
--
-- The hosted_google_connections ALTERs add discovery status metadata so a
-- failed or unavailable resource-discovery run is persisted distinctly from
-- an empty inventory without touching existing grants.

CREATE TABLE IF NOT EXISTS hosted_oauth_states (
  state_hash TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  clerk_user_id TEXT NOT NULL,
  clerk_session_id TEXT NOT NULL,
  intent TEXT NOT NULL CHECK (intent IN ('connect', 'reconnect')),
  code_verifier TEXT NOT NULL,
  nonce TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  scopes_json TEXT NOT NULL DEFAULT '[]',
  services_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  FOREIGN KEY (tenant_id, connection_id)
    REFERENCES hosted_google_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS hosted_oauth_states_tenant_created_idx
  ON hosted_oauth_states (tenant_id, created_at);

ALTER TABLE hosted_google_connections ADD COLUMN discovery_state TEXT NOT NULL DEFAULT '';
ALTER TABLE hosted_google_connections ADD COLUMN discovery_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE hosted_google_connections ADD COLUMN discovery_checked_at TEXT;
