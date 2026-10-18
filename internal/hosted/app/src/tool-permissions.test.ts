import { beforeEach, afterEach, describe, it, expect } from "vitest";
import {
  createTestDatabase,
  applyMigrationSql,
  type SQLiteD1Adapter,
} from "../../state/src/sqlite-adapter.js";
import { loadMigrationSql } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import { EXECUTABLE_TOOLS, saveToolPermissions, toolPermissionView } from "./tool-permissions.js";
import type { ConnectSession } from "./connect.js";
import { policyTools } from "./mcp.js";

describe("explicit executable tool permissions", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;
  let session: ConnectSession;
  let connectionId: string;
  beforeEach(async () => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    applyMigrationSql(db, loadMigrationSql("0005_tool_grants.sql"));
    repo = new HostedRepository(db);
    const tenant = await repo.bootstrapTenant("owner");
    session = { tenantId: tenant.id, userId: "owner", sessionId: "session" };
    const connection = await repo.createConnection(
      tenant.id,
      "google-sub",
      "owner@example.com",
      "",
      "[]",
    );
    connectionId = connection.id;
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      "docs",
      "selected-file",
      "file",
      "Doc",
      true,
      "{}",
    );
  });
  afterEach(() => db.close());

  it("persists the entire expanded fixed catalogue without granting unknown or duplicate tools", async () => {
    const updates = EXECUTABLE_TOOLS.map((tool) => ({
      toolName: tool.name,
      enabled: false,
      allowDestructive: false,
    }));
    await saveToolPermissions(repo, session, connectionId, updates);
    expect(await repo.listToolGrants(session.tenantId, connectionId)).toHaveLength(
      EXECUTABLE_TOOLS.length,
    );
    expect(
      (await toolPermissionView(repo, session, connectionId)).tools.every((tool) => !tool.enabled),
    ).toBe(true);
    await expect(
      saveToolPermissions(repo, session, connectionId, [...updates, updates[0]!]),
    ).rejects.toMatchObject({ code: "invalid_request" });
  });

  it("does not turn enabled assets/consent into write authority; saves exact choices", async () => {
    const view = await toolPermissionView(repo, session, connectionId);
    expect(view.tools).toHaveLength(167);
    expect(view.tools.every((tool) => !tool.enabled)).toBe(true);
    const tenant = (await repo.getTenant(session.tenantId))!;
    expect(await policyTools(repo, tenant)).toEqual([]);
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "docs_write", enabled: true, allowDestructive: false },
    ]);
    expect((await policyTools(repo, tenant)).map((tool) => tool.name)).toEqual(["docs_write"]);
    expect(
      (await toolPermissionView(repo, session, connectionId)).tools.find(
        (tool) => tool.name === "docs_write",
      ),
    ).toMatchObject({ enabled: true, allowDestructive: false });
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "docs_write", enabled: false, allowDestructive: false },
    ]);
    expect(await policyTools(repo, tenant)).toEqual([]);
  });

  it("exposes read-only Drive label schemas with account grants, not synthetic files", async () => {
    const tenant = (await repo.getTenant(session.tenantId))!;
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "drivelabels_get", enabled: true, allowDestructive: false },
    ]);
    expect((await policyTools(repo, tenant)).map((tool) => tool.name)).toEqual(["drivelabels_get"]);
    await expect(
      saveToolPermissions(repo, session, connectionId, [
        { toolName: "drivelabels_get", enabled: true, allowDestructive: true },
      ]),
    ).rejects.toMatchObject({ code: "invalid_request" });
  });

  it("exposes account Tasks only with an explicit matching tool grant, without fake resources", async () => {
    const tenant = (await repo.getTenant(session.tenantId))!;
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "tasks_lists_create", enabled: true, allowDestructive: false },
    ]);
    expect((await policyTools(repo, tenant)).map((tool) => tool.name)).toEqual([
      "tasks_lists_create",
    ]);
    expect(
      (await repo.listResourceGrants(tenant.id, connectionId)).some(
        (grant) => grant.service === "tasks",
      ),
    ).toBe(false);
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "tasks_lists_create", enabled: false, allowDestructive: false },
    ]);
    expect(await policyTools(repo, tenant)).toEqual([]);
  });

  it("requires a selected Slides resource and an explicit account tool grant", async () => {
    const tenant = (await repo.getTenant(session.tenantId))!;
    await saveToolPermissions(repo, session, connectionId, [
      { toolName: "slides_update_notes", enabled: true, allowDestructive: true },
    ]);
    expect(await policyTools(repo, tenant)).toEqual([]);
    await repo.upsertResourceGrant(
      tenant.id,
      connectionId,
      "slides",
      "selected-deck",
      "file",
      "Deck",
      true,
      "{}",
    );
    expect((await policyTools(repo, tenant)).map((tool) => tool.name)).toEqual([
      "slides_update_notes",
    ]);
    const view = await toolPermissionView(repo, session, connectionId);
    expect(view.tools.find((tool) => tool.name === "slides_update_notes")).toMatchObject({
      write: true,
      enabled: true,
      allowDestructive: true,
    });
    expect(view.tools.find((tool) => tool.name === "slides_list_slides")).toMatchObject({
      write: false,
      enabled: false,
      allowDestructive: false,
    });
  });

  it("rejects unsupported/wildcard/billing/foreign authority and validates whole batches", async () => {
    for (const grants of [
      [{ toolName: "gmail_search", enabled: true, allowDestructive: false }],
      [{ toolName: "*", enabled: true, allowDestructive: false }],
      [{ toolName: "docs_get", enabled: true, allowDestructive: true }],
      [{ toolName: "docs_write", enabled: true, allowDestructive: false, allowBillable: true }],
      [
        { toolName: "docs_write", enabled: true, allowDestructive: false },
        { toolName: "unknown_tool", enabled: true, allowDestructive: false },
      ],
    ]) {
      await expect(saveToolPermissions(repo, session, connectionId, grants)).rejects.toMatchObject({
        code: "invalid_request",
      });
      expect(await repo.listToolGrants(session.tenantId, connectionId)).toEqual([]);
    }
    const foreign = await repo.bootstrapTenant("foreign");
    await expect(
      saveToolPermissions(repo, { ...session, tenantId: foreign.id }, connectionId, []),
    ).rejects.toMatchObject({ code: "connection_not_found" });
  });
});
