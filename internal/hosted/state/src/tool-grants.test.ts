import { beforeEach, afterEach, describe, it, expect } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("account-scoped tool grants", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;
  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    applyMigrationSql(db, loadMigrationSql("0005_tool_grants.sql"));
    repo = new HostedRepository(db);
  });
  afterEach(() => db.close());

  it("persists separate tool/write choices per account and cascades on disconnect", async () => {
    const tenant = await repo.bootstrapTenant("owner");
    const first = await repo.createConnection(tenant.id, "one", "one@example.com", "", "[]");
    const second = await repo.createConnection(tenant.id, "two", "two@example.com", "", "[]");
    expect(await repo.listToolGrants(tenant.id, first.id)).toEqual([]);
    await repo.saveToolGrants(tenant.id, first.id, [
      {
        service: "docs",
        toolName: "docs_write",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    expect(await repo.listToolGrants(tenant.id, first.id)).toMatchObject([
      { toolName: "docs_write", enabled: true, allowDestructive: false },
    ]);
    expect(await repo.listToolGrants(tenant.id, second.id)).toEqual([]);
    await repo.saveToolGrants(tenant.id, first.id, [
      {
        service: "docs",
        toolName: "docs_write",
        enabled: false,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    expect((await repo.listToolGrants(tenant.id, first.id))[0]!.enabled).toBe(false);
    await repo.deleteConnection(tenant.id, first.id);
    expect(await repo.listToolGrants(tenant.id, first.id)).toEqual([]);
  });

  it("rejects foreign account writes in repository and SQL foreign keys", async () => {
    const a = await repo.bootstrapTenant("a");
    const b = await repo.bootstrapTenant("b");
    const connection = await repo.createConnection(a.id, "sub-a", "a@example.com", "", "[]");
    await expect(
      repo.saveToolGrants(b.id, connection.id, [
        {
          service: "docs",
          toolName: "docs_write",
          enabled: true,
          allowDestructive: true,
          allowBillable: false,
        },
      ]),
    ).rejects.toThrow("owned connection");
    await expect(
      db
        .prepare(
          "INSERT INTO hosted_tool_grants (tenant_id, connection_id, service, tool_name, updated_at) VALUES (?1, ?2, 'docs', 'docs_write', 'now')",
        )
        .bind(b.id, connection.id)
        .run(),
    ).rejects.toThrow(/FOREIGN KEY/i);
    expect(await repo.listToolGrants(b.id, connection.id)).toEqual([]);
  });

  it("migration preserves only existing GA4 reads and does not re-enable explicitly disabled tools", async () => {
    const tenant = await repo.bootstrapTenant("existing-owner");
    const connection = await repo.createConnection(tenant.id, "sub", "a@example.com", "", "[]");
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      "analytics",
      "properties/1",
      "property",
      "P",
      true,
      "{}",
    );
    applyMigrationSql(db, loadMigrationSql("0005_tool_grants.sql"));
    const grants = await repo.listToolGrants(tenant.id, connection.id);
    expect(grants.map((grant) => grant.toolName)).toEqual([
      "analytics_datastreams_list",
      "analytics_properties_get",
      "analytics_report",
    ]);
    expect(
      grants.every((grant) => grant.enabled && !grant.allowDestructive && !grant.allowBillable),
    ).toBe(true);
    await repo.saveToolGrants(tenant.id, connection.id, [
      {
        service: "analytics",
        toolName: "analytics_report",
        enabled: false,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    applyMigrationSql(db, loadMigrationSql("0005_tool_grants.sql"));
    expect(
      (await repo.listToolGrants(tenant.id, connection.id)).find(
        (grant) => grant.toolName === "analytics_report",
      )!.enabled,
    ).toBe(false);
  });
});
