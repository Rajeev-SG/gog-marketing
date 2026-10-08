-- Hosted v1 fixed-window request rate counters.
--
-- Tenant counters are keyed by the internal tenant UUID. IP counters are
-- keyed by a SHA-256 digest so operators can aggregate obvious abuse without
-- storing a raw customer IP address. The table intentionally has no tenant
-- foreign key because the IP limit must also cover requests that have not yet
-- resolved a tenant.

CREATE TABLE IF NOT EXISTS hosted_rate_limit_counters (
  scope_kind TEXT NOT NULL CHECK (scope_kind IN ('tenant', 'ip_hash')),
  scope_key TEXT NOT NULL,
  period TEXT NOT NULL,
  value INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (scope_kind, scope_key, period)
);

CREATE INDEX IF NOT EXISTS hosted_rate_limit_counters_period_idx
  ON hosted_rate_limit_counters (period);
