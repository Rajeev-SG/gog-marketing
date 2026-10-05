import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("credential material (ciphertext/nonce/key_version only)", () => {
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

  it("credential table has no plaintext or token columns", async () => {
    const columns =
      (await db.prepare("PRAGMA table_info(hosted_connection_credentials)").all<{ name: string }>())
        .results ?? [];
    const names = columns.map((c) => c.name);

    // Allowed columns — only ciphertext, nonce, and key_version carry credential material.
    const allowed = new Set([
      "tenant_id",
      "connection_id",
      "ciphertext",
      "nonce",
      "key_version",
      "created_at",
      "updated_at",
    ]);
    for (const col of names) {
      expect(allowed.has(col), `unexpected column: ${col}`).toBe(true);
    }

    // Explicitly verify no forbidden plaintext/token columns exist.
    for (const forbidden of [
      "token",
      "refresh_token",
      "access_token",
      "plaintext",
      "secret",
      "credential",
    ]) {
      expect(names.some((n) => n.toLowerCase().includes(forbidden))).toBe(false);
    }
  });

  it("stores ciphertext/nonce/key_version and can read them back", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const conn = await repo.createConnection(tenant.id, "sub_a", "a@gmail.com", "", "[]");

    const ciphertext = new Uint8Array([0x01, 0x02, 0x03, 0x04]);
    const nonce = new Uint8Array([0xaa, 0xbb, 0xcc]);
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext,
      nonce,
      keyVersion: 2,
    });

    const stored = await repo.getCredential(tenant.id, conn.id);
    expect(stored).not.toBeNull();
    expect(stored?.ciphertext).toEqual(ciphertext);
    expect(stored?.nonce).toEqual(nonce);
    expect(stored?.keyVersion).toBe(2);
  });

  it("upserting a credential replaces the previous material", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const conn = await repo.createConnection(tenant.id, "sub_a", "a@gmail.com", "", "[]");

    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext: new Uint8Array([1]),
      nonce: new Uint8Array([2]),
      keyVersion: 1,
    });

    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext: new Uint8Array([3, 4]),
      nonce: new Uint8Array([5, 6]),
      keyVersion: 2,
    });

    const stored = await repo.getCredential(tenant.id, conn.id);
    expect(stored?.ciphertext).toEqual(new Uint8Array([3, 4]));
    expect(stored?.keyVersion).toBe(2);

    // Verify only one row exists.
    const count =
      (
        await db
          .prepare("SELECT COUNT(*) as n FROM hosted_connection_credentials")
          .all<{ n: number }>()
      ).results ?? [];
    expect(count[0]?.n).toBe(1);
  });

  it("supports key rotation: existing key_version remains until replaced", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const connA = await repo.createConnection(tenant.id, "sub_a", "a@gmail.com", "", "[]");
    const connB = await repo.createConnection(tenant.id, "sub_b", "b@gmail.com", "", "[]");

    // Store different key versions for different connections (rotation in progress).
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: connA.id,
      ciphertext: new Uint8Array([1]),
      nonce: new Uint8Array([2]),
      keyVersion: 1,
    });
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: connB.id,
      ciphertext: new Uint8Array([3]),
      nonce: new Uint8Array([4]),
      keyVersion: 2,
    });

    const credA = await repo.getCredential(tenant.id, connA.id);
    const credB = await repo.getCredential(tenant.id, connB.id);
    expect(credA?.keyVersion).toBe(1);
    expect(credB?.keyVersion).toBe(2);
  });

  it("removes credentials when the connection is deleted (cascade)", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const conn = await repo.createConnection(tenant.id, "sub_a", "a@gmail.com", "", "[]");
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext: new Uint8Array([1]),
      nonce: new Uint8Array([2]),
      keyVersion: 1,
    });

    await repo.deleteConnection(tenant.id, conn.id);
    const stored = await repo.getCredential(tenant.id, conn.id);
    expect(stored).toBeNull();
  });

  it("cross-tenant credential read returns null even with the same connectionId", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_user_a");
    const tenantB = await repo.bootstrapTenant("clerk_user_b");
    const connA = await repo.createConnection(tenantA.id, "sub_a", "a@gmail.com", "", "[]");
    await repo.upsertCredential({
      tenantId: tenantA.id,
      connectionId: connA.id,
      ciphertext: new Uint8Array([1]),
      nonce: new Uint8Array([2]),
      keyVersion: 1,
    });

    const fromB = await repo.getCredential(tenantB.id, connA.id);
    expect(fromB).toBeNull();
  });

  it("credential material never appears in audit event detail", async () => {
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    const conn = await repo.createConnection(tenant.id, "sub_a", "a@gmail.com", "", "[]");
    const rawToken = "ya29.super-secret-token";
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: conn.id,
      ciphertext: new Uint8Array(Buffer.from(rawToken)),
      nonce: new Uint8Array([2]),
      keyVersion: 1,
    });

    // Write an audit event with safe metadata only.
    await repo.appendAudit({
      id: "audit1",
      tenantId: tenant.id,
      connectionId: conn.id,
      actorClerkUserId: "clerk_user_a",
      action: "google.analytics.read",
      result: "allow",
      detailJson: JSON.stringify({ resource: "ga4-123", error: "none" }),
      latencyMs: 42,
    });

    const audit = await repo.listAudit(tenant.id, 10);
    expect(audit).toHaveLength(1);
    // The audit row must not contain the raw token anywhere.
    const rowStr = JSON.stringify(audit[0]);
    expect(rowStr).not.toContain(rawToken);
  });
});

describe("audit schema safety", () => {
  let db: SQLiteD1Adapter;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
  });

  afterEach(() => {
    db.close();
  });

  it("audit table has no credential or token columns", async () => {
    const columns =
      (await db.prepare("PRAGMA table_info(hosted_audit_events)").all<{ name: string }>())
        .results ?? [];
    const names = columns.map((c) => c.name);

    for (const forbidden of [
      "token",
      "oauth_code",
      "code_verifier",
      "refresh",
      "ciphertext",
      "nonce",
      "credential",
      "secret",
      "raw_payload",
    ]) {
      expect(names.some((n) => n.toLowerCase().includes(forbidden))).toBe(false);
    }
  });
});
