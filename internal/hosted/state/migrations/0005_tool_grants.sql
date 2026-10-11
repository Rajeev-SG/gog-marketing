-- Explicit per-account tool authority. Resource selection and OAuth consent
-- alone never authorize writes or automatically enable future tools.
CREATE TABLE IF NOT EXISTS hosted_tool_grants (
  tenant_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  service TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
  allow_destructive INTEGER NOT NULL DEFAULT 0 CHECK (allow_destructive IN (0, 1)),
  allow_billable INTEGER NOT NULL DEFAULT 0 CHECK (allow_billable IN (0, 1)),
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tenant_id, connection_id, tool_name),
  FOREIGN KEY (tenant_id, connection_id)
    REFERENCES hosted_google_connections (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS hosted_tool_grants_service_idx
  ON hosted_tool_grants (tenant_id, connection_id, service, enabled);

-- Preserve only the three existing GA4 read capabilities for already-enabled
-- properties. No writes, other services, wildcards or future tools are seeded.
INSERT OR IGNORE INTO hosted_tool_grants
  (tenant_id, connection_id, service, tool_name, enabled, allow_destructive, allow_billable, updated_at)
SELECT DISTINCT r.tenant_id, r.connection_id, 'analytics', tools.tool_name, 1, 0, 0, r.updated_at
FROM hosted_resource_grants r
JOIN hosted_google_connections c ON c.tenant_id = r.tenant_id AND c.id = r.connection_id
JOIN (
  SELECT 'analytics_properties_get' AS tool_name
  UNION ALL SELECT 'analytics_report'
  UNION ALL SELECT 'analytics_datastreams_list'
) tools
WHERE r.service = 'analytics' AND r.enabled = 1 AND c.status = 'active';
