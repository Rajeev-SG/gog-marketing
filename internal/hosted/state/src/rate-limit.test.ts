import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("fixed-window request rate counters", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0003_rate_limit_counters.sql"));
    repo = new HostedRepository(db);
  });

  afterEach(() => {
    db.close();
  });

  it("increments per scope and window", async () => {
    expect(await repo.incrementRateLimit("tenant", "tenant-a", "2026-10-08T12:00")).toBe(1);
    expect(await repo.incrementRateLimit("tenant", "tenant-a", "2026-10-08T12:00")).toBe(2);
    expect(await repo.incrementRateLimit("tenant", "tenant-a", "2026-10-08T12:01")).toBe(1);
    expect(await repo.incrementRateLimit("tenant", "tenant-b", "2026-10-08T12:00")).toBe(1);
  });

  it("keeps hashed IP counters independent from tenant counters", async () => {
    const ipHash = "a".repeat(64);
    expect(await repo.incrementRateLimit("ip_hash", ipHash, "2026-10-08T12:00")).toBe(1);
    expect(await repo.incrementRateLimit("ip_hash", ipHash, "2026-10-08T12:00")).toBe(2);
    expect(await repo.incrementRateLimit("tenant", "tenant-a", "2026-10-08T12:00")).toBe(1);

    await expect(
      repo.incrementRateLimit("ip_hash", "198.51.100.10", "2026-10-08T12:00"),
    ).rejects.toThrow(/invalid scope key/i);
  });

  it("returns exact running values for concurrent increments", async () => {
    const values = await Promise.all(
      Array.from({ length: 10 }, () =>
        repo.incrementRateLimit("tenant", "tenant-race", "2026-10-08T12:00"),
      ),
    );

    expect([...values].sort((a, b) => a - b)).toEqual(
      Array.from({ length: 10 }, (_, index) => index + 1),
    );
  });
});
