import { beforeEach, describe, expect, it } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("audit storage boundary (safe metadata only)", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    repo = new HostedRepository(db);
  });

  it("stores allowlisted, correctly typed detail metadata", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    await repo.appendAudit({
      id: "a1",
      tenantId: tenant.id,
      connectionId: "",
      actorClerkUserId: "clerk_audit",
      action: "google.analytics.read",
      result: "allow",
      detailJson: JSON.stringify({ resource: "ga4-123", count: 3, enabled: true }),
    });

    const events = await repo.listAudit(tenant.id, 10);
    expect(events).toHaveLength(1);
    expect(JSON.parse(events[0]!.detailJson)).toEqual({
      resource: "ga4-123",
      count: 3,
      enabled: true,
    });
  });

  it("rejects an unsafe write attempt without storing it or echoing the value", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    const unsafe = JSON.stringify({ token: "ya29.SUPERSECRET-VALUE" });

    let message = "";
    try {
      await repo.appendAudit({
        id: "a2",
        tenantId: tenant.id,
        connectionId: "",
        actorClerkUserId: "clerk_audit",
        action: "oauth.exchange",
        result: "error",
        detailJson: unsafe,
      });
      expect.unreachable("unsafe audit detail must be rejected");
    } catch (err) {
      message = err instanceof Error ? err.message : String(err);
    }

    expect(message).toBeTruthy();
    expect(message).not.toContain("ya29.SUPERSECRET-VALUE");
    expect(message).not.toContain(unsafe);

    const events = await repo.listAudit(tenant.id, 10);
    expect(events).toHaveLength(0);
  });

  it("rejects unknown keys, nested material, arrays, and invalid types", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    const base = {
      tenantId: tenant.id,
      connectionId: "",
      actorClerkUserId: "clerk_audit",
      action: "tool.call",
      result: "allow" as const,
    };

    const unsafeInputs = [
      JSON.stringify({ unknown_key: "x" }),
      JSON.stringify({ meta: { deep: "material" } }),
      JSON.stringify(["array"]),
      JSON.stringify({ resource: { nested: true } }),
      JSON.stringify({ count: "3" }),
      JSON.stringify({ error: "x".repeat(300) }),
      "not-json",
    ];
    for (const detailJson of unsafeInputs) {
      await expect(repo.appendAudit({ id: "a3", ...base, detailJson })).rejects.toThrow(
        /unsafe|must be/,
      );
    }

    expect(await repo.listAudit(tenant.id, 10)).toHaveLength(0);
  });

  it("rejects credential content under allowed detail keys without echo or storage", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    const marker = "SYNTHETIC-AUDIT-MARKER";
    for (const key of ["error", "resource", "operation"]) {
      for (const value of [
        "refresh_token=" + marker,
        'access_token: "' + marker + '"',
        "Authorization: Bearer " + marker,
        "Bearer " + marker,
        "ya29." + marker,
        "client_secret=" + marker,
        "code_verifier=" + marker,
      ]) {
        await expect(
          repo.appendAudit({
            id: "unsafe",
            tenantId: tenant.id,
            connectionId: "",
            actorClerkUserId: "clerk_audit",
            action: "tool.call",
            result: "error",
            detailJson: JSON.stringify({ [key]: value }),
          }),
        ).rejects.toMatchObject({ message: "appendAudit: unsafe audit detail rejected" });
        expect(await repo.listAudit(tenant.id, 10)).toHaveLength(0);
      }
    }
  });

  it("rejects credential content in stored audit labels too", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    for (const key of ["id", "action", "actorClerkUserId"]) {
      await expect(
        repo.appendAudit({
          id: "unsafe-label",
          tenantId: tenant.id,
          connectionId: "",
          actorClerkUserId: "clerk_audit",
          action: "tool.call",
          result: "error",
          detailJson: JSON.stringify({ error: "internal_error" }),
          [key]: "refresh_token=SYNTHETIC-LABEL-MARKER",
        }),
      ).rejects.toMatchObject({ message: "appendAudit: unsafe audit detail rejected" });
    }
    expect(await repo.listAudit(tenant.id, 10)).toHaveLength(0);
  });

  it("preserves safe error codes and identifiers containing token", async () => {
    const tenant = await repo.bootstrapTenant("clerk_audit");
    const detail = {
      error: "permission_denied",
      resource: "token-report-123",
      operation: "token_inventory.read",
    };
    await repo.appendAudit({
      id: "safe",
      tenantId: tenant.id,
      connectionId: "",
      actorClerkUserId: "clerk_audit",
      action: "tool.call",
      result: "error",
      detailJson: JSON.stringify(detail),
    });
    expect(JSON.parse((await repo.listAudit(tenant.id, 10))[0]!.detailJson)).toEqual(detail);
    await expect(
      repo.appendAudit({
        id: "raw-error",
        tenantId: tenant.id,
        connectionId: "",
        actorClerkUserId: "clerk_audit",
        action: "tool.call",
        result: "error",
        detailJson: JSON.stringify({ error: "provider exception with raw context" }),
      }),
    ).rejects.toMatchObject({ message: "appendAudit: unsafe audit detail rejected" });
  });

  it("rejects an actor belonging to another tenant without storing a row", async () => {
    const tenant = await repo.bootstrapTenant("clerk_owner");
    await repo.bootstrapTenant("clerk_foreign");
    await expect(
      repo.appendAudit({
        id: "foreign-actor",
        tenantId: tenant.id,
        connectionId: "",
        actorClerkUserId: "clerk_foreign",
        action: "tool.call",
        result: "allow",
        detailJson: "{}",
      }),
    ).rejects.toThrow("appendAudit: actor not owned by tenant");
    expect(await repo.listAudit(tenant.id, 10)).toHaveLength(0);
  });

  it("rejects audit rows that claim a foreign connection as tenant-owned", async () => {
    const tenantA = await repo.bootstrapTenant("clerk_a");
    const tenantB = await repo.bootstrapTenant("clerk_b");
    const connB = await repo.createConnection(tenantB.id, "sub_b", "b@gmail.com", "", "[]");

    await expect(
      repo.appendAudit({
        id: "a4",
        tenantId: tenantA.id,
        connectionId: connB.id,
        actorClerkUserId: "clerk_a",
        action: "tool.call",
        result: "allow",
        detailJson: JSON.stringify({ resource: "x" }),
      }),
    ).rejects.toThrow(/connection not owned/i);

    const events = await repo.listAudit(tenantA.id, 10);
    expect(events).toHaveLength(0);
  });
});
