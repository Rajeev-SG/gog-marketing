import { beforeEach, afterEach, describe, it, expect } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("bound one-use Picker sessions", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;
  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    applyMigrationSql(db, loadMigrationSql("0006_picker_sessions.sql"));
    repo = new HostedRepository(db);
  });
  afterEach(() => db.close());
  it("binds account, tenant, Clerk session, subject, expiry and consumption", async () => {
    const tenant = await repo.bootstrapTenant("owner");
    const connection = await repo.createConnection(
      tenant.id,
      "google-a",
      "a@example.com",
      "",
      "[]",
    );
    const input = {
      sessionHash: "hash",
      tenantId: tenant.id,
      connectionId: connection.id,
      clerkSessionId: "sid-a",
      googleSubject: "google-a",
      expiresAt: "2026-10-11T12:05:00.000Z",
    };
    await repo.createPickerSession(input);
    const take = { ...input, now: "2026-10-11T12:00:00.000Z" };
    for (const changed of [
      { tenantId: "foreign" },
      { connectionId: "other" },
      { clerkSessionId: "sid-b" },
      { googleSubject: "google-b" },
      { now: "2026-10-11T12:05:01.000Z" },
    ])
      expect(await repo.consumePickerSession({ ...take, ...changed })).toBe(false);
    expect(await repo.consumePickerSession(take)).toBe(true);
    expect(await repo.consumePickerSession(take)).toBe(false);
  });
  it("foreign connections fail compound FK and disconnect removes sessions", async () => {
    const owner = await repo.bootstrapTenant("a");
    const foreign = await repo.bootstrapTenant("b");
    const connection = await repo.createConnection(owner.id, "a", "a@example.com", "", "[]");
    const row = {
      sessionHash: "hash",
      tenantId: owner.id,
      connectionId: connection.id,
      clerkSessionId: "s",
      googleSubject: "a",
      expiresAt: "2099-01-01",
    };
    await expect(repo.createPickerSession({ ...row, tenantId: foreign.id })).rejects.toThrow(
      /FOREIGN KEY/i,
    );
    await repo.createPickerSession(row);
    await repo.deleteConnection(owner.id, connection.id);
    expect(await repo.consumePickerSession({ ...row, now: "2026-10-11" })).toBe(false);
  });
});
