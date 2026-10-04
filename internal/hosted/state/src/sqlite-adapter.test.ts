import { describe, expect, it, beforeEach } from "vitest";
import { createTestDatabase, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { applyMigrationSql } from "./sqlite-adapter.js";

describe("test SQLite adapter (D1-compatible batch)", () => {
  let db: SQLiteD1Adapter;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
  });

  it("batch executes bound statements and preserves every binding parameter", async () => {
    const results = await db.batch([
      db
        .prepare(
          "INSERT INTO hosted_tenants (id, clerk_user_id, status, created_at, updated_at) VALUES (?1, ?2, 'active', ?3, ?3)",
        )
        .bind("t1", "clerk_batch", "2026-01-01"),
      db.prepare("SELECT clerk_user_id FROM hosted_tenants WHERE id = ?1").bind("t1"),
    ]);

    expect(results.map((r) => r.success)).toEqual([true, true]);
    expect((results[1]?.results ?? []) as { clerk_user_id: string }[]).toEqual([
      { clerk_user_id: "clerk_batch" },
    ]);
  });
});
