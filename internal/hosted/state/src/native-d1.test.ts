/**
 * Native D1 tests (Miniflare workerd).
 *
 * These run against the real D1 implementation — not the node:sqlite test
 * adapter — so they exercise actual D1 statement handling, BLOB result types
 * (BLOB reads cross the Miniflare proxy as number[]), migration application,
 * and D1 write serialisation under concurrency.
 */
import { afterEach, describe, expect, it } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";
import type { Database } from "./types.js";

const WORKER_NAME = "hosted-state-native";

let mf: Miniflare | undefined;

afterEach(async () => {
  await mf?.dispose();
  mf = undefined;
});

async function createNativeDb(): Promise<D1Database> {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: WORKER_NAME,
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "hosted-state-native-test" },
          script: "export default {};",
          modules: true,
        },
      ],
    }),
  );
  return mf.getD1Database("DB");
}

async function applyMigrations(db: D1Database): Promise<void> {
  const statements = splitMigrationStatements(loadMigrationSql("0001_initial_schema.sql"));
  for (const statement of statements) {
    const result = await db.batch([db.prepare(statement)]);
    if (!result[0]?.success) throw new Error(`migration statement failed: ${statement}`);
  }
}

describe("native D1 (workerd)", () => {
  it("applies the migration from empty and is idempotent", async () => {
    const db = await createNativeDb();
    await applyMigrations(db);

    const tables = await db
      .prepare(
        "SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'hosted_%' ORDER BY name",
      )
      .all<{ name: string }>();
    const names = (tables.results ?? []).map((r) => r.name);
    expect(names).toEqual([
      "hosted_audit_events",
      "hosted_connection_credentials",
      "hosted_google_connections",
      "hosted_quota_counters",
      "hosted_resource_grants",
      "hosted_tenants",
    ]);

    // Re-applying must not fail (IF NOT EXISTS everywhere).
    await expect(applyMigrations(db)).resolves.toBeUndefined();
  });

  it("stores BLOBs and normalises native D1 number[] reads to byte arrays", async () => {
    const db = await createNativeDb();
    await applyMigrations(db);
    const repo = new HostedRepository(db as unknown as Database);

    const tenant = await repo.bootstrapTenant("clerk_native_blob");
    const conn = await repo.createConnection(tenant.id, "sub_native", "n@gmail.com", "", "[]");
    const ciphertext = new Uint8Array([0x00, 0x01, 0xfe, 0xff, 0x42]);
    const nonce = new Uint8Array([0x13, 0x37, 0x00]);
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext,
      nonce,
      keyVersion: 5,
    });

    // Raw D1 read: BLOB columns cross the native D1 proxy as number[].
    const raw = await db
      .prepare("SELECT ciphertext, nonce, key_version FROM hosted_connection_credentials")
      .first<{ ciphertext: number[]; nonce: number[]; key_version: number }>();
    expect(Array.isArray(raw?.ciphertext)).toBe(true);
    expect(Array.from(raw?.ciphertext ?? [])).toEqual(Array.from(ciphertext));

    // Repository read must return a stable byte-array shape.
    const stored = await repo.getCredential(tenant.id, conn.id);
    expect(stored).not.toBeNull();
    expect(stored?.ciphertext).toBeInstanceOf(Uint8Array);
    expect(Array.from(stored?.ciphertext ?? [])).toEqual(Array.from(ciphertext));
    expect(Array.from(stored?.nonce ?? [])).toEqual(Array.from(nonce));
    expect(stored?.keyVersion).toBe(5);
  });

  it("batch keeps every binding parameter on native D1", async () => {
    const db = await createNativeDb();
    await applyMigrations(db);

    const results = await db.batch([
      db
        .prepare(
          "INSERT INTO hosted_tenants (id, clerk_user_id, status, created_at, updated_at) VALUES (?1, ?2, 'active', ?3, ?3)",
        )
        .bind("t1", "clerk_batch", "2026-01-01"),
      db.prepare("SELECT clerk_user_id FROM hosted_tenants WHERE id = ?1").bind("t1"),
    ]);

    expect(results.map((r) => r.success)).toEqual([true, true]);
    const read = (results[1]?.results ?? []) as { clerk_user_id: string }[];
    expect(read[0]?.clerk_user_id).toBe("clerk_batch");
  });

  it("serialises concurrent bootstrap callers to one stable tenant", async () => {
    const db = await createNativeDb();
    await applyMigrations(db);
    const repo = new HostedRepository(db as unknown as Database);

    const callers = 8;
    const tenants = await Promise.all(
      Array.from({ length: callers }, () => repo.bootstrapTenant("clerk_native_race")),
    );
    expect(new Set(tenants.map((t) => t.id)).size).toBe(1);

    const count = await db
      .prepare("SELECT COUNT(*) AS n FROM hosted_tenants WHERE clerk_user_id = ?1")
      .bind("clerk_native_race")
      .first<{ n: number }>();
    expect(count?.n).toBe(1);
  });

  it("returns exact running values 1..N for concurrent quota increments", async () => {
    const db = await createNativeDb();
    await applyMigrations(db);
    const repo = new HostedRepository(db as unknown as Database);

    const tenant = await repo.bootstrapTenant("clerk_native_quota");
    const callers = 16;
    const values = await Promise.all(
      Array.from({ length: callers }, () => repo.incrementQuota(tenant.id, "2026-10", "native", 1)),
    );

    expect([...values].sort((a, b) => a - b)).toEqual(
      Array.from({ length: callers }, (_, i) => i + 1),
    );
  });
});
