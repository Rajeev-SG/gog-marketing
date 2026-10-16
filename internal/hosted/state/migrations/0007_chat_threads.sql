-- Index only. Native Chat Durable Objects remain the sole transcript store.
CREATE TABLE IF NOT EXISTS hosted_chat_threads (
  tenant_id TEXT NOT NULL REFERENCES hosted_tenants(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  conversation TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 100),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  purged INTEGER NOT NULL DEFAULT 0 CHECK(purged IN (0,1)),
  PRIMARY KEY(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS hosted_chat_threads_owner_updated ON hosted_chat_threads(tenant_id, deleted_at, updated_at);
