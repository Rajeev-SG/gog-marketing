import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("quota counters (atomic increments)", () => {
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

  it("increments from zero to the delta on first write", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const value = await repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", 1);
    expect(value).toBe(1);
  });

  it("returns the running total after sequential increments", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");

    const v1 = await repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", 1);
    expect(v1).toBe(1);
    const v2 = await repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", 1);
    expect(v2).toBe(2);
    const v3 = await repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", 1);
    expect(v3).toBe(3);
  });

  it("supports non-unit deltas and resets correctly per period", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");

    const v1 = await repo.incrementQuota(tenant.id, "2024-12-01", "api_calls", 5);
    expect(v1).toBe(5);
    const v2 = await repo.incrementQuota(tenant.id, "2024-12-01", "api_calls", 3);
    expect(v2).toBe(8);
    const v3 = await repo.incrementQuota(tenant.id, "2024-12-02", "api_calls", 1);
    expect(v3).toBe(1);
  });

  it("separates counters for different tenants", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");

    await repo.incrementQuota(tenantA.id, "2024-12-01", "mcp_calls", 10);
    await repo.incrementQuota(tenantB.id, "2024-12-01", "mcp_calls", 1);

    const quotaA = await repo.getQuota(tenantA.id, "2024-12-01", "mcp_calls");
    expect(quotaA?.value).toBe(10);

    const quotaB = await repo.getQuota(tenantB.id, "2024-12-01", "mcp_calls");
    expect(quotaB?.value).toBe(1);
  });

  it("rejects quota writes for a nonexistent tenant via FK", async () => {
    await expect(
      db
        .prepare(
          "INSERT INTO hosted_quota_counters (tenant_id, period, counter, value, updated_at) VALUES ('no-such', '2024', 'x', 0, '2024-01-01')",
        )
        .run(),
    ).rejects.toThrow(/FOREIGN KEY/i);
  });

  it("getQuota returns null for a counter that has not been created yet", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const result = await repo.getQuota(tenant.id, "2024-12-01", "never_written");
    expect(result).toBeNull();
  });

  it("many rapid increments produce the correct final value", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const n = 50;
    let last = 0;
    for (let i = 0; i < n; i++) {
      last = await repo.incrementQuota(tenant.id, "2024-12-01", "rapid", 1);
    }
    expect(last).toBe(n);

    const stored = await repo.getQuota(tenant.id, "2024-12-01", "rapid");
    expect(stored?.value).toBe(n);
  });

  it("concurrent increments each return their own running value", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_race");
    const callers = 16;

    const values = await Promise.all(
      Array.from({ length: callers }, () =>
        repo.incrementQuota(tenant.id, "2024-12-01", "race", 1),
      ),
    );

    expect([...values].sort((a, b) => a - b)).toEqual(
      Array.from({ length: callers }, (_, i) => i + 1),
    );
  });

  it("rejects negative and fractional increments without writing", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");

    await expect(repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", -1)).rejects.toThrow();
    await expect(repo.incrementQuota(tenant.id, "2024-12-01", "mcp_calls", 0.5)).rejects.toThrow();

    const stored = await repo.getQuota(tenant.id, "2024-12-01", "mcp_calls");
    expect(stored).toBeNull();
  });
});
