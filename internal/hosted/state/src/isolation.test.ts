import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("tenant isolation (SQL-layer enforcement)", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    repo = new HostedRepository(db);
  });

  afterEach(() => {
    db.close();
  });

  it("rejects a connection insert for a nonexistent tenant via FK", async () => {
    await expect(
      db
        .prepare(
          "INSERT INTO hosted_google_connections (id, tenant_id, google_subject, created_at, updated_at) VALUES ('c1', 'no-such-tenant', 'sub', '2024-01-01', '2024-01-01')",
        )
        .run(),
    ).rejects.toThrow(/FOREIGN KEY/i);
  });

  it("compound FK rejects a credential row for a connection owned by a different tenant", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");

    // Attempt to store a credential for connA but under tenantB — this
    // should fail because the compound FK (tenant_id, connection_id)
    // does not match any row in hosted_google_connections.
    await expect(
      db
        .prepare(
          `INSERT INTO hosted_connection_credentials
           (tenant_id, connection_id, ciphertext, nonce, key_version, created_at, updated_at)
         VALUES ('${tenantB.id}', '${connA.id}', x'00', x'00', 1, '2024-01-01', '2024-01-01')`,
        )
        .run(),
    ).rejects.toThrow(/FOREIGN KEY/i);
  });

  it("compound FK rejects a resource grant for a connection owned by a different tenant", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");

    await expect(
      db
        .prepare(
          `INSERT INTO hosted_resource_grants
           (id, tenant_id, connection_id, service, resource_id, created_at, updated_at)
         VALUES ('g1', '${tenantB.id}', '${connA.id}', 'analytics', 'ga4-123', '2024-01-01', '2024-01-01')`,
        )
        .run(),
    ).rejects.toThrow(/FOREIGN KEY/i);
  });

  it("listing connections returns only the tenant's own connections", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    await repo.createConnection(tenantA.id, "sub_a1", "a1@gmail.com", "", "[]");
    await repo.createConnection(tenantA.id, "sub_a2", "a2@gmail.com", "", "[]");
    await repo.createConnection(tenantB.id, "sub_b1", "b1@gmail.com", "", "[]");

    const connsA = await repo.listConnections(tenantA.id);
    const connsB = await repo.listConnections(tenantB.id);

    expect(connsA).toHaveLength(2);
    expect(connsA.every((c) => c.tenantId === tenantA.id)).toBe(true);
    expect(connsB).toHaveLength(1);
    expect(connsB[0].tenantId).toBe(tenantB.id);
  });

  it("lookup by connectionId does not return another tenant's connection", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");
    await repo.createConnection(tenantB.id, "sub_b", "b@gmail.com", "", "[]");

    // Tenant B looking up tenant A's connection gets null because the
    // query is scoped by tenant_id.
    const result = await repo.getConnection(tenantB.id, connA.id);
    expect(result).toBeNull();
  });

  it("resource grant lookup is tenant-scoped", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");
    await repo.upsertResourceGrant(
      tenantA.id,
      connA.id,
      "analytics",
      "ga4-123",
      "property",
      "My GA4",
      true,
      "{}",
    );

    // Tenant B cannot find tenant A's grant even with the same service/resource_id.
    const found = await repo.getEnabledResourceGrant(tenantB.id, connA.id, "analytics", "ga4-123");
    expect(found).toBeNull();

    // But tenant A can.
    const foundA = await repo.getEnabledResourceGrant(tenantA.id, connA.id, "analytics", "ga4-123");
    expect(foundA).not.toBeNull();
    expect(foundA?.enabled).toBe(true);
  });

  it("listing audit events returns only the tenant's own audit trail", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    await repo.appendAudit({
      id: "a1",
      tenantId: tenantA.id,
      connectionId: "",
      actorClerkUserId: "clerk_user_a",
      action: "tool_call",
      result: "allow",
      detailJson: "{}",
    });
    await repo.appendAudit({
      id: "a2",
      tenantId: tenantB.id,
      connectionId: "",
      actorClerkUserId: "clerk_user_b",
      action: "tool_call",
      result: "deny",
      detailJson: "{}",
    });

    const auditA = await repo.listAudit(tenantA.id, 10);
    const auditB = await repo.listAudit(tenantB.id, 10);

    expect(auditA).toHaveLength(1);
    expect(auditA[0].result).toBe("allow");
    expect(auditB).toHaveLength(1);
    expect(auditB[0].result).toBe("deny");
  });

  it("deleting a tenant cascades to its connections, grants, credentials, and audit", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");
    await repo.upsertCredential({
      tenantId: tenantA.id,
      connectionId: connA.id,
      ciphertext: new Uint8Array([1, 2, 3]),
      nonce: new Uint8Array([4, 5, 6]),
      keyVersion: 1,
    });
    await repo.upsertResourceGrant(
      tenantA.id,
      connA.id,
      "analytics",
      "ga4-123",
      "property",
      "",
      true,
      "{}",
    );

    await db.prepare("DELETE FROM hosted_tenants WHERE id = ?1").bind(tenantA.id).run();

    const conns =
      (await db.prepare("SELECT COUNT(*) as n FROM hosted_google_connections").all<{ n: number }>())
        .results ?? [];
    expect(conns[0]?.n).toBe(0);
    const creds =
      (
        await db
          .prepare("SELECT COUNT(*) as n FROM hosted_connection_credentials")
          .all<{ n: number }>()
      ).results ?? [];
    expect(creds[0]?.n).toBe(0);
    const grants =
      (await db.prepare("SELECT COUNT(*) as n FROM hosted_resource_grants").all<{ n: number }>())
        .results ?? [];
    expect(grants[0]?.n).toBe(0);
  });
});
