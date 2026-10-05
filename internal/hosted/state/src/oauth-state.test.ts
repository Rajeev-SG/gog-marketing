import { beforeEach, afterEach, describe, expect, it } from "vitest";
import { createTestDatabase, applyMigrationSql, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";

describe("hosted OAuth state binding (#62)", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;
  let tenantId: string;
  let connectionId: string;

  beforeEach(async () => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
    applyMigrationSql(db, loadMigrationSql("0002_oauth_states_discovery.sql"));
    repo = new HostedRepository(db);
    const tenant = await repo.bootstrapTenant("clerk_user_a");
    tenantId = tenant.id;
    const connection = await repo.createConnection(tenantId, "pending-abc", "", "", "[]");
    connectionId = connection.id;
  });

  afterEach(() => {
    db.close();
  });

  function baseInput(overrides?: Partial<Parameters<HostedRepository["createOAuthState"]>[0]>) {
    return {
      stateHash: "hash-1",
      tenantId,
      connectionId,
      clerkUserId: "clerk_user_a",
      clerkSessionId: "sess_a",
      intent: "connect" as const,
      codeVerifier: "verifier",
      nonce: "nonce",
      redirectUri: "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback",
      scopes: ["openid", "email"],
      services: ["analytics"],
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      ...overrides,
    };
  }

  it("takes a state exactly once and records the consumption", async () => {
    await repo.createOAuthState(baseInput());
    const first = await repo.takeOAuthState(tenantId, "hash-1", "sess_a");
    expect(first.kind).toBe("taken");
    if (first.kind !== "taken") return;
    expect(first.state.connectionId).toBe(connectionId);
    expect(first.state.scopes).toEqual(["openid", "email"]);
    const second = await repo.takeOAuthState(tenantId, "hash-1", "sess_a");
    expect(second.kind).toBe("replayed");
  });

  it("rejects a foreign tenant without consuming the binding", async () => {
    await repo.createOAuthState(baseInput());
    const other = await repo.bootstrapTenant("clerk_user_b");
    const result = await repo.takeOAuthState(other.id, "hash-1", "sess_b");
    expect(result.kind).toBe("wrong_tenant");
    const stillUsable = await repo.takeOAuthState(tenantId, "hash-1", "sess_a");
    expect(stillUsable.kind).toBe("taken");
  });

  it("rejects a sibling session of the same user without consuming the binding", async () => {
    await repo.createOAuthState(baseInput());
    const sibling = await repo.takeOAuthState(tenantId, "hash-1", "sess_b");
    expect(sibling.kind).toBe("wrong_session");
    const owner = await repo.takeOAuthState(tenantId, "hash-1", "sess_a");
    expect(owner.kind).toBe("taken");
  });

  it("reports expired states and unknown hashes distinctly", async () => {
    await repo.createOAuthState(
      baseInput({
        stateHash: "hash-expired",
        expiresAt: new Date(Date.now() - 1000).toISOString(),
      }),
    );
    expect((await repo.takeOAuthState(tenantId, "hash-expired", "sess_a")).kind).toBe("expired");
    expect((await repo.takeOAuthState(tenantId, "hash-missing", "sess_a")).kind).toBe("unknown");
  });

  it("cascades state deletion with the intended connection", async () => {
    await repo.createOAuthState(baseInput());
    await repo.deleteConnection(tenantId, connectionId);
    const result = await repo.takeOAuthState(tenantId, "hash-1", "sess_a");
    expect(result.kind).toBe("unknown");
  });

  it("finds identity conflicts across connections but never itself", async () => {
    await repo.updateConnectionIdentity(tenantId, connectionId, {
      googleSubject: "sub-1",
      email: "Work@Example.com",
      displayName: "Work",
      grantedScopesJson: "[]",
    });
    const other = await repo.createConnection(tenantId, "pending-2", "", "", "[]");
    const conflict = await repo.findIdentityConflict(
      tenantId,
      other.id,
      "sub-1",
      "unrelated@x.test",
    );
    expect(conflict?.id).toBe(connectionId);
    const conflictByEmail = await repo.findIdentityConflict(
      tenantId,
      other.id,
      "other-sub",
      "work@example.com",
    );
    expect(conflictByEmail?.id).toBe(connectionId);
    expect(
      await repo.findIdentityConflict(tenantId, connectionId, "sub-1", "work@example.com"),
    ).toBeNull();
  });

  it("persists distinct discovery outcomes per connection", async () => {
    await repo.updateConnectionDiscovery(
      tenantId,
      connectionId,
      "unavailable",
      "runner_not_configured",
      "2026-10-05T00:00:00.000Z",
    );
    let connection = await repo.getConnection(tenantId, connectionId);
    expect(connection?.discoveryState).toBe("unavailable");
    expect(connection?.discoveryDetail).toBe("runner_not_configured");
    await repo.updateConnectionDiscovery(
      tenantId,
      connectionId,
      "empty",
      "no_resources",
      "2026-10-05T00:01:00.000Z",
    );
    connection = await repo.getConnection(tenantId, connectionId);
    expect(connection?.discoveryState).toBe("empty");
  });
});
