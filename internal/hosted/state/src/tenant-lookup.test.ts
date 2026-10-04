import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("tenant lookup (idempotent Clerk→tenant bootstrap)", () => {
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

  it("creates a new tenant on first bootstrap", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_abc");
    expect(tenant.id).toBeTruthy();
    expect(tenant.clerkUserId).toBe("clerk_user_abc");
    expect(tenant.status).toBe("active");
  });

  it("returns the same tenant on repeated bootstrap (idempotent)", async () => {
    const first = await repo.bootstrapTenant("clerk_user_abc");
    const second = await repo.bootstrapTenant("clerk_user_abc");
    expect(second.id).toBe(first.id);
    expect(second.createdAt).toBe(first.createdAt);

    // Verify no duplicate rows exist.
    const count =
      (
        await db
          .prepare("SELECT COUNT(*) as n FROM hosted_tenants WHERE clerk_user_id = ?1")
          .bind("clerk_user_abc")
          .all<{ n: number }>()
      ).results ?? [];
    expect(count[0]?.n).toBe(1);
  });

  it("concurrent bootstrap callers for the same Clerk user all resolve to one stable tenant", async () => {
    const callers = 8;
    const tenants = await Promise.all(
      Array.from({ length: callers }, () => repo.bootstrapTenant("clerk_user_race")),
    );

    const ids = new Set(tenants.map((t) => t.id));
    expect(ids.size).toBe(1);
    expect(tenants.every((t) => t.clerkUserId === "clerk_user_race")).toBe(true);

    const count =
      (
        await db
          .prepare("SELECT COUNT(*) as n FROM hosted_tenants WHERE clerk_user_id = ?1")
          .bind("clerk_user_race")
          .all<{ n: number }>()
      ).results ?? [];
    expect(count[0]?.n).toBe(1);
  });

  it("resolves by Clerk user ID after bootstrap", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_abc");
    const found = await repo.getTenantByClerkId("clerk_user_abc");
    expect(found).not.toBeNull();
    expect(found?.id).toBe(tenant.id);
  });

  it("different Clerk users get different tenants", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");
    expect(tenantA.id).not.toBe(tenantB.id);
  });

  it("getTenant returns null for a nonexistent tenant ID", async () => {
    const result = await repo.getTenant("nonexistent-id");
    expect(result).toBeNull();
  });
});

describe("multiple Google accounts per tenant", () => {
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

  it("a tenant can connect multiple distinct Google accounts", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");

    const connA = await repo.createConnection(
      tenant.id,
      "sub_account_a",
      "a@gmail.com",
      "Account A",
      "[]",
    );
    const connB = await repo.createConnection(
      tenant.id,
      "sub_account_b",
      "b@gmail.com",
      "Account B",
      "[]",
    );

    expect(connA.id).not.toBe(connB.id);
    expect(connA.googleSubject).not.toBe(connB.googleSubject);

    const conns = await repo.listConnections(tenant.id);
    expect(conns).toHaveLength(2);
  });

  it("rejects a duplicate Google subject for the same tenant", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    await repo.createConnection(tenant.id, "sub_dup", "a@gmail.com", "", "[]");

    await expect(
      repo.createConnection(tenant.id, "sub_dup", "a@gmail.com", "", "[]"),
    ).rejects.toThrow();
  });

  it("the same Google subject can exist under different tenants", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    const connA = await repo.createConnection(
      tenantA.id,
      "same_subject",
      "shared@gmail.com",
      "",
      "[]",
    );
    const connB = await repo.createConnection(
      tenantB.id,
      "same_subject",
      "shared@gmail.com",
      "",
      "[]",
    );
    expect(connA.id).not.toBe(connB.id);
  });
});
