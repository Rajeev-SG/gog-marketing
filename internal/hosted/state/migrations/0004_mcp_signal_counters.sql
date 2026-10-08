-- Hosted MCP tenant-independent observable signal counters.
--
-- Authentication can fail before a tenant is known, and protocol errors can
-- occur before request processing resolves one. These aggregate rows provide
-- secret-free operator visibility without storing bearer values, request
-- bodies, source IPs, or tenant identifiers.

CREATE TABLE IF NOT EXISTS hosted_mcp_signal_counters (
  signal TEXT NOT NULL,
  period TEXT NOT NULL,
  value INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (signal, period)
);

CREATE INDEX IF NOT EXISTS hosted_mcp_signal_counters_period_idx
  ON hosted_mcp_signal_counters (period);
