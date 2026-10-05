import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";

describe("D1 migrations", () => {
  let db: SQLiteD1Adapter;

  beforeEach(() => {
    db = createTestDatabase();
  });

  afterEach(() => {
    db.close();
  });

  it("applies from empty and creates all hosted tables", async () => {
    const sql = loadMigrationSql("0001_initial_schema.sql");
    applyMigrationSql(db, sql);

    const tables =
      (
        await db
          .prepare(
            "SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'hosted_%' ORDER BY name",
          )
          .all<{ name: string }>()
      ).results ?? [];

    const names = tables.map((r) => r.name);
    expect(names).toContain("hosted_tenants");
    expect(names).toContain("hosted_google_connections");
    expect(names).toContain("hosted_connection_credentials");
    expect(names).toContain("hosted_resource_grants");
    expect(names).toContain("hosted_audit_events");
    expect(names).toContain("hosted_quota_counters");
  });

  it("is idempotent: reapplying the migration succeeds without error", () => {
    const sql = loadMigrationSql("0001_initial_schema.sql");
    applyMigrationSql(db, sql);
    expect(() => applyMigrationSql(db, sql)).not.toThrow();
  });

  it("is deterministic: applying twice produces the same schema", async () => {
    const sql = loadMigrationSql("0001_initial_schema.sql");
    applyMigrationSql(db, sql);
    const first = await getSchema(db);
    applyMigrationSql(db, sql);
    const second = await getSchema(db);
    expect(second).toEqual(first);
  });

  it("indexes the tenant/connection/grant lookup paths", async () => {
    const sql = loadMigrationSql("0001_initial_schema.sql");
    applyMigrationSql(db, sql);

    const indexes =
      (
        await db
          .prepare(
            "SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'hosted_%' ORDER BY name",
          )
          .all<{ name: string }>()
      ).results ?? [];

    const names = indexes.map((r) => r.name);
    expect(names).toContain("hosted_google_connections_tenant_status_idx");
    expect(names).toContain("hosted_resource_grants_tenant_conn_idx");
    expect(names).toContain("hosted_audit_events_tenant_created_idx");
  });

  it("applies the OAuth state / discovery migration and keeps #61 schema intact", async () => {
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    applyMigrationSql(db, loadMigrationSql("0002_oauth_states_discovery.sql"));

    const tables =
      (
        await db
          .prepare("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'hosted_%'")
          .all<{ name: string }>()
      ).results ?? [];
    expect(tables.map((r) => r.name)).toContain("hosted_oauth_states");

    const columns =
      (await db.prepare("PRAGMA table_info(hosted_google_connections)").all<{ name: string }>())
        .results ?? [];
    const names = columns.map((r) => r.name);
    expect(names).toContain("discovery_state");
    expect(names).toContain("discovery_detail");
    expect(names).toContain("discovery_checked_at");

    const stateColumns =
      (await db.prepare("PRAGMA table_info(hosted_oauth_states)").all<{ name: string }>())
        .results ?? [];
    expect(stateColumns.map((r) => r.name)).toContain("clerk_session_id");

    const indexes =
      (
        await db
          .prepare("SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'hosted_%'")
          .all<{ name: string }>()
      ).results ?? [];
    expect(indexes.map((r) => r.name)).toContain("hosted_oauth_states_tenant_created_idx");
  });
});

async function getSchema(db: SQLiteD1Adapter): Promise<string[]> {
  const result =
    (
      await db
        .prepare(
          "SELECT name, type, sql FROM sqlite_master WHERE name LIKE 'hosted_%' ORDER BY name, type",
        )
        .all<{ name: string; type: string; sql: string | null }>()
    ).results ?? [];
  return result.map((r) => `${r.type}:${r.name}:${r.sql ?? ""}`);
}
