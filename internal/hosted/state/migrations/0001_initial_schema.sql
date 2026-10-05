-- Hosted v1 D1 initial schema.
--
-- D1 enforces foreign keys (equivalent to PRAGMA foreign_keys = on), so the
-- parent table must be created before its children. All tables are prefixed
-- with "hosted_" to avoid collision with the tracking worker's tables.
--
-- Every child table carries tenant_id and uses a compound foreign key to
-- enforce ownership at the SQL layer, not just in query filtering.

CREATE TABLE IF NOT EXISTS hosted_tenants (
  id TEXT PRIMARY KEY,
  clerk_user_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS hosted_google_connections (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  google_subject TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  granted_scopes_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'needs_reconnect', 'revoked')),
  last_error TEXT NOT NULL DEFAULT '',
  last_validated_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (tenant_id, google_subject),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id) REFERENCES hosted_tenants (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS hosted_google_connections_tenant_status_idx
  ON hosted_google_connections (tenant_id, status);

CREATE TABLE IF NOT EXISTS hosted_connection_credentials (
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  ciphertext BLOB NOT NULL,
  nonce BLOB NOT NULL,
  key_version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tenant_id, connection_id),
  FOREIGN KEY (tenant_id, connection_id)
    REFERENCES hosted_google_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS hosted_resource_grants (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  service TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  discovered_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (tenant_id, connection_id, service, resource_id),
  FOREIGN KEY (tenant_id, connection_id)
    REFERENCES hosted_google_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS hosted_resource_grants_tenant_conn_idx
  ON hosted_resource_grants (tenant_id, connection_id, service);

CREATE TABLE IF NOT EXISTS hosted_audit_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL DEFAULT '',
  actor_clerk_user_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  result TEXT NOT NULL CHECK (result IN ('allow', 'deny', 'error', 'info')),
  detail_json TEXT NOT NULL DEFAULT '{}',
  latency_ms INTEGER,
  created_at TEXT NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES hosted_tenants (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS hosted_audit_events_tenant_created_idx
  ON hosted_audit_events (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS hosted_quota_counters (
  tenant_id TEXT NOT NULL,
  period TEXT NOT NULL,
  counter TEXT NOT NULL,
  value INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tenant_id, period, counter),
  FOREIGN KEY (tenant_id) REFERENCES hosted_tenants (id) ON DELETE CASCADE
);
