CREATE TABLE IF NOT EXISTS users (
  id text PRIMARY KEY,
  email text NOT NULL UNIQUE,
  external_subject text NOT NULL DEFAULT '',
  display_name text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS organizations (
  id text PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS memberships (
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('owner', 'admin')),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, organization_id)
);

CREATE TABLE IF NOT EXISTS google_connections (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  google_email text NOT NULL DEFAULT '',
  google_subject text NOT NULL DEFAULT '',
  oauth_client_id text NOT NULL DEFAULT '',
  services_json text NOT NULL DEFAULT '[]',
  requested_scopes_json text NOT NULL DEFAULT '[]',
  granted_scopes_json text NOT NULL DEFAULT '[]',
  status text NOT NULL,
  token_secret_ref text NOT NULL DEFAULT '',
  last_validated_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS resource_grants (
  id text PRIMARY KEY,
  connection_id text NOT NULL REFERENCES google_connections(id) ON DELETE CASCADE,
  organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  service text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  display_name text NOT NULL DEFAULT '',
  parent text NOT NULL DEFAULT '',
  metadata_json text NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT false,
  discovered_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (connection_id, resource_id)
);

CREATE TABLE IF NOT EXISTS audit_events (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  acting_user_id text NOT NULL DEFAULT '',
  connection_id text NOT NULL DEFAULT '',
  action text NOT NULL,
  result text NOT NULL,
  detail text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_states (
  state text PRIMARY KEY,
  organization_id text NOT NULL,
  connection_id text NOT NULL,
  code_verifier text NOT NULL,
  redirect_uri text NOT NULL,
  scopes_json text NOT NULL DEFAULT '[]',
  nonce text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_events_org_created_idx ON audit_events (organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS resource_grants_connection_idx ON resource_grants (connection_id, service);
