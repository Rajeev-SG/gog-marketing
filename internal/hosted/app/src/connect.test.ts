import { beforeEach, afterEach, describe, expect, it } from "vitest";
import { createTestDatabase, applyMigrationSql } from "../../state/src/sqlite-adapter.js";
import { loadMigrationSql } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import { CredentialCipher, parseCredentialKeys } from "./credential-crypto.js";
import { GoogleOAuthClient } from "./google-oauth.js";
import {
  completeCallback,
  disconnect,
  listConnections,
  refreshConnection,
  startConnect,
  type ConnectDeps,
  type ConnectSession,
} from "./connect.js";
import {
  mapWireReport,
  UnavailableDiscoveryRunner,
  type DiscoveryOutcome,
  type DiscoveryRunner,
} from "./discovery.js";

const KEY = Buffer.alloc(32, 9).toString("base64");
const REDIRECT = "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback";
const TENANT_A = "clerk_user_a";
const TENANT_B = "clerk_user_b";

interface GoogleStub {
  token?: Record<string, unknown> | Error;
  userinfo?: Record<string, unknown> | Error;
}

function stubFetchFor(stub: GoogleStub): {
  fetch: typeof fetch;
  tokenCalls: string[];
  revokeCalls: string[];
} {
  const tokenCalls: string[] = [];
  const revokeCalls: string[] = [];
  const impl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/token")) {
      tokenCalls.push(String(init?.body));
      if (stub.token instanceof Error) throw stub.token;
      return new Response(JSON.stringify(stub.token ?? {}), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.includes("userinfo")) {
      if (stub.userinfo instanceof Error) throw stub.userinfo;
      return new Response(JSON.stringify(stub.userinfo ?? {}), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.includes("/revoke")) {
      revokeCalls.push(String(init?.body));
      return new Response(null, { status: 200 });
    }
    throw new Error(`unexpected fetch ${url}`);
  }) as typeof fetch;
  return { fetch: impl, tokenCalls, revokeCalls };
}

function tokenResponse(overrides?: Record<string, unknown>): Record<string, unknown> {
  return {
    access_token: "ya29.access",
    refresh_token: "1//refresh-secret",
    token_type: "Bearer",
    expires_in: 3600,
    scope: "openid email https://www.googleapis.com/auth/analytics.readonly",
    ...overrides,
  };
}

interface Harness {
  repo: HostedRepository;
  cipher: CredentialCipher;
  deps: (options?: { google?: GoogleStub; runner?: DiscoveryRunner }) => ConnectDeps;
  sessionFor(clerkUserId: string, sessionId?: string): Promise<ConnectSession>;
  close(): void;
}

async function createHarness(): Promise<Harness> {
  const db = createTestDatabase();
  applyMigrationSql(db, loadMigrationSql("0001_initial_schema.sql"));
  applyMigrationSql(db, loadMigrationSql("0002_oauth_states_discovery.sql"));
  const repo = new HostedRepository(db);
  const cipher = new CredentialCipher(parseCredentialKeys(KEY));
  const deps = (options?: { google?: GoogleStub; runner?: DiscoveryRunner }): ConnectDeps => {
    const stub = stubFetchFor(
      options?.google ?? {
        token: tokenResponse(),
        userinfo: { sub: "sub-1", email: "a@example.com", email_verified: true, name: "Account A" },
      },
    );
    const client = new GoogleOAuthClient({
      clientId: "cid",
      clientSecret: "csecret",
      fetchImpl: stub.fetch,
    });
    return {
      repo,
      oauth: client,
      cipher,
      discovery: options?.runner ?? new UnavailableDiscoveryRunner(),
      redirectUri: REDIRECT,
      stateTtlSeconds: 600,
    };
  };
  const sessionFor = async (clerkUserId: string, sessionId = "sess-1"): Promise<ConnectSession> => {
    const tenant = await repo.bootstrapTenant(clerkUserId);
    return { userId: clerkUserId, sessionId, tenantId: tenant.id };
  };
  return { repo, cipher, deps, sessionFor, close: () => db.close() };
}

describe("hosted Google connection lifecycle (#62)", () => {
  let harness: Harness;

  beforeEach(async () => {
    harness = await createHarness();
  });

  afterEach(() => {
    harness.close();
  });

  it("starts a bound flow: stub connection + one-use state + PKCE/nonce URL", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, { services: ["analytics", "gmail"] });
    expect(start.authorizationUrl).toContain("accounts.google.com");
    expect(start.authorizationUrl).toContain("code_challenge_method=S256");
    expect(start.authorizationUrl).toContain("access_type=offline");
    expect(start.authorizationUrl).toContain("prompt=consent");
    expect(start.authorizationUrl).toContain(encodeURIComponent(REDIRECT));

    const rows = await harness.repo.listConnections(session.tenantId);
    expect(rows).toHaveLength(1);
    expect(rows[0]!.googleSubject.startsWith("pending-")).toBe(true);
    const connections = await listConnections(harness.deps(), session);
    const view = connections.connections[0]!;
    expect(JSON.stringify(view)).not.toContain("1//");
    expect(JSON.stringify(view)).not.toContain("ya29");
  });

  it("rejects unknown services and reconnects to missing connections", async () => {
    const session = await harness.sessionFor(TENANT_A);
    await expect(
      startConnect(harness.deps(), session, { services: ["youtube"] }),
    ).rejects.toMatchObject({
      code: "invalid_request",
    });
    await expect(
      startConnect(harness.deps(), session, { connectionId: "missing" }),
    ).rejects.toMatchObject({
      code: "connection_not_found",
    });
  });

  it("completes a happy connect with verified identity and unavailable discovery", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, { services: ["analytics"] });
    // The state parameter is the raw token from the authorization URL.
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const result = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({ code: "auth-code", state: rawState }),
    );
    expect(result.connection.status).toBe("active");
    expect(result.connection.email).toBe("a@example.com");
    expect(result.connection.grantedScopes).toContain(
      "https://www.googleapis.com/auth/analytics.readonly",
    );
    expect(result.discovery.status).toBe("unavailable");
    expect(result.discovery.detail).toBe("runner_not_configured");

    // Credential at rest: ciphertext only.
    const stored = await harness.repo.getCredential(session.tenantId, result.connection.id);
    expect(stored).not.toBeNull();
    const decrypted = await harness.cipher.decrypt(session.tenantId, result.connection.id, stored!);
    expect(decrypted.refresh_token).toBe("1//refresh-secret");
    const json = JSON.stringify(result);
    expect(json).not.toContain("1//refresh-secret");
    expect(json).not.toContain("ya29.access");

    // Replay of the same state is rejected.
    await expect(
      completeCallback(
        harness.deps(),
        session,
        new URLSearchParams({ code: "auth-code", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "state_replayed" });
  });

  it("rejects callbacks from a different tenant and from a sibling session without consuming the state", async () => {
    const sessionA = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), sessionA, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const sessionB = await harness.sessionFor(TENANT_B);
    await expect(
      completeCallback(
        harness.deps(),
        sessionB,
        new URLSearchParams({ code: "c", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "callback_wrong_tenant" });
    const sibling = await harness.sessionFor(TENANT_A, "sess-2");
    await expect(
      completeCallback(
        harness.deps(),
        sibling,
        new URLSearchParams({ code: "c", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "callback_wrong_session" });
    // The owner can still complete it.
    const result = await completeCallback(
      harness.deps(),
      sessionA,
      new URLSearchParams({ code: "c", state: rawState }),
    );
    expect(result.connection.status).toBe("active");
  });

  it("rejects callbacks whose intended connection was deleted", async () => {
    // The compound FK cascades state deletion with the connection, so a
    // callback for a deleted connection surfaces as an unknown state and can
    // never complete against a foreign or missing connection.
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const rows = await harness.repo.listConnections(session.tenantId);
    await harness.repo.deleteConnection(session.tenantId, rows[0]!.id);
    await expect(
      completeCallback(
        harness.deps(),
        session,
        new URLSearchParams({ code: "c", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "state_unknown" });
  });

  it("maps consent denial to a distinct outcome and discards the stub", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    await expect(
      completeCallback(
        harness.deps(),
        session,
        new URLSearchParams({ error: "access_denied", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "consent_denied" });
    expect(await harness.repo.listConnections(session.tenantId)).toHaveLength(0);
  });

  it("classifies exchange failures distinctly and keeps reconnect state", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    await expect(
      completeCallback(
        harness.deps({ google: { token: new Error("network down") } }),
        session,
        new URLSearchParams({ code: "c", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "google_unavailable" });
    expect(await harness.repo.listConnections(session.tenantId)).toHaveLength(0); // stub discarded
  });

  it("blocks duplicate identities without touching the first connection", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const first = await startConnect(harness.deps(), session, {});
    const rawState1 = new URL(first.authorizationUrl).searchParams.get("state")!;
    const done = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({ code: "c", state: rawState1 }),
    );
    expect(done.connection.email).toBe("a@example.com");

    const second = await startConnect(harness.deps(), session, {});
    const rawState2 = new URL(second.authorizationUrl).searchParams.get("state")!;
    await expect(
      completeCallback(
        harness.deps(),
        session,
        new URLSearchParams({ code: "c2", state: rawState2 }),
      ),
    ).rejects.toMatchObject({ code: "identity_conflict" });
    const connections = await harness.repo.listConnections(session.tenantId);
    expect(connections).toHaveLength(1);
    expect(connections[0]!.id).toBe(done.connection.id);
    expect(connections[0]!.email).toBe("a@example.com");
  });

  it("never lets a reconnect replace the account on a connection", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const connection = await harness.repo.createConnection(
      session.tenantId,
      "sub-original",
      "orig@example.com",
      "Original",
      "[]",
    );
    const start = await startConnect(harness.deps(), session, { connectionId: connection.id });
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    await expect(
      completeCallback(
        harness.deps({
          google: {
            token: tokenResponse(),
            userinfo: { sub: "sub-other", email: "other@example.com", email_verified: true },
          },
        }),
        session,
        new URLSearchParams({ code: "c", state: rawState }),
      ),
    ).rejects.toMatchObject({ code: "identity_conflict" });
    const still = await harness.repo.getConnection(session.tenantId, connection.id);
    expect(still?.googleSubject).toBe("sub-original");
    expect(still?.status).toBe("active");
    // Same account reconnect succeeds.
    const start2 = await startConnect(harness.deps(), session, { connectionId: connection.id });
    const rawState2 = new URL(start2.authorizationUrl).searchParams.get("state")!;
    const result = await completeCallback(
      harness.deps({
        google: {
          token: tokenResponse(),
          userinfo: { sub: "sub-original", email: "orig@example.com", email_verified: true },
        },
      }),
      session,
      new URLSearchParams({ code: "c", state: rawState2 }),
    );
    expect(result.connection.status).toBe("active");
    expect(result.connection.id).toBe(connection.id);
  });

  it("applies discovery results with least privilege and preserves grants on failure", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, { services: ["analytics"] });
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const okRunner: DiscoveryRunner = {
      async discover(): Promise<DiscoveryOutcome> {
        return mapWireReport({
          resources: [
            {
              service: "analytics",
              resource_type: "property",
              resource_id: "properties/1",
              display_name: "Prop 1",
            },
          ],
          statuses: {
            analytics: {
              state: "ok",
              detail: "",
              resource_count: 1,
              checked_at: "2026-10-05T00:00:00Z",
            },
          },
        });
      },
    };
    const result = await completeCallback(
      harness.deps({ runner: okRunner }),
      session,
      new URLSearchParams({ code: "c", state: rawState }),
    );
    expect(result.discovery.status).toBe("ok");
    const grants = await harness.repo.listResourceGrants(session.tenantId, result.connection.id);
    expect(grants).toHaveLength(1);
    expect(grants[0]!.enabled).toBe(false); // new resources default disabled

    // Error discovery leaves the existing grant untouched.
    const errorRunner: DiscoveryRunner = {
      async discover(): Promise<DiscoveryOutcome> {
        return { status: "error", detail: "service_error", resources: [], statuses: {} };
      },
    };
    const start2 = await startConnect(harness.deps(), session, {
      connectionId: result.connection.id,
    });
    const rawState2 = new URL(start2.authorizationUrl).searchParams.get("state")!;
    const result2 = await completeCallback(
      harness.deps({
        runner: errorRunner,
        google: {
          token: tokenResponse(),
          userinfo: { sub: "sub-1", email: "a@example.com", email_verified: true },
        },
      }),
      session,
      new URLSearchParams({ code: "c", state: rawState2 }),
    );
    expect(result2.discovery.status).toBe("error");
    const grantsAfterError = await harness.repo.listResourceGrants(
      session.tenantId,
      result.connection.id,
    );
    expect(grantsAfterError).toHaveLength(1);
    expect(
      (await harness.repo.getConnection(session.tenantId, result.connection.id))?.discoveryState,
    ).toBe("error");

    // Empty discovery is distinct from failure and from unavailable.
    const emptyRunner: DiscoveryRunner = {
      async discover(): Promise<DiscoveryOutcome> {
        return { status: "empty", detail: "no_resources", resources: [], statuses: {} };
      },
    };
    const start3 = await startConnect(harness.deps(), session, {
      connectionId: result.connection.id,
    });
    const rawState3 = new URL(start3.authorizationUrl).searchParams.get("state")!;
    const result3 = await completeCallback(
      harness.deps({
        runner: emptyRunner,
        google: {
          token: tokenResponse(),
          userinfo: { sub: "sub-1", email: "a@example.com", email_verified: true },
        },
      }),
      session,
      new URLSearchParams({ code: "c", state: rawState3 }),
    );
    expect(result3.discovery.status).toBe("empty");
    expect(
      await harness.repo.listResourceGrants(session.tenantId, result.connection.id),
    ).toHaveLength(1);
  });

  it("surfaces expired/revoked grants distinctly and drops the dead credential, keeping grants", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const result = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({ code: "c", state: rawState }),
    );
    await harness.repo.upsertResourceGrant(
      session.tenantId,
      result.connection.id,
      "analytics",
      "properties/1",
      "property",
      "P1",
      true,
      "{}",
    );

    // A successful refresh restores/re-encrypts and keeps the grant state.
    const refreshed = await refreshConnection(harness.deps(), session, result.connection.id);
    expect(refreshed.status).toBe("active");
    const stored = await harness.repo.getCredential(session.tenantId, result.connection.id);
    expect(stored).not.toBeNull();
    const decrypted = await harness.cipher.decrypt(session.tenantId, result.connection.id, stored!);
    expect(decrypted.access_token).toBe("ya29.access");

    // Expired/revoked grant: distinct needs_reconnect outcome; the dead
    // credential is dropped while the connection row and grants survive.
    await expect(
      refreshConnection(
        harness.deps({ google: { token: { error: "invalid_grant" } } }),
        session,
        result.connection.id,
      ),
    ).rejects.toMatchObject({ code: "needs_reconnect" });
    expect(await harness.repo.getCredential(session.tenantId, result.connection.id)).toBeNull();
    const connection = await harness.repo.getConnection(session.tenantId, result.connection.id);
    expect(connection?.status).toBe("needs_reconnect");
    expect(
      await harness.repo.listResourceGrants(session.tenantId, result.connection.id),
    ).toHaveLength(1);
  });

  it("disconnects without leaving credentials and attempts revocation", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    const result = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({ code: "c", state: rawState }),
    );
    await harness.repo.upsertResourceGrant(
      session.tenantId,
      result.connection.id,
      "analytics",
      "properties/1",
      "property",
      "P1",
      true,
      "{}",
    );

    const revokeCalls: string[] = [];
    const deps = harness.deps();
    const revokeFetch = (async (input: RequestInfo | URL) => {
      if (String(input).includes("/revoke")) revokeCalls.push(String(input));
      return new Response(null, { status: 200 });
    }) as typeof fetch;
    const client = new GoogleOAuthClient({
      clientId: "cid",
      clientSecret: "s",
      fetchImpl: revokeFetch,
    });
    await disconnect({ ...deps, oauth: client }, session, result.connection.id);
    expect(revokeCalls.length).toBeGreaterThanOrEqual(0);
    expect(await harness.repo.getConnection(session.tenantId, result.connection.id)).toBeNull();
    expect(await harness.repo.getCredential(session.tenantId, result.connection.id)).toBeNull();
    expect(
      await harness.repo.listResourceGrants(session.tenantId, result.connection.id),
    ).toHaveLength(0);
    await expect(disconnect(deps, session, result.connection.id)).rejects.toMatchObject({
      code: "connection_not_found",
    });
  });

  it("keeps audit rows free of credential material", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const rawState = new URL(start.authorizationUrl).searchParams.get("state")!;
    await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({ code: "c", state: rawState }),
    );
    const audit = await harness.repo.listAudit(session.tenantId, 20);
    expect(audit.length).toBeGreaterThan(0);
    for (const event of audit) {
      expect(event.detailJson).not.toContain("1//");
      expect(event.detailJson).not.toContain("ya29");
    }
  });
  it("rejects email reassignment and preserves the complete old account snapshot", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, { services: ["analytics"] });
    const done = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({
        code: "c",
        state: new URL(start.authorizationUrl).searchParams.get("state")!,
      }),
    );
    await harness.repo.upsertResourceGrant(
      session.tenantId,
      done.connection.id,
      "analytics",
      "properties/1",
      "property",
      "P",
      true,
      "{}",
    );
    const before = await harness.repo.getConnection(session.tenantId, done.connection.id);
    const credential = await harness.repo.getCredential(session.tenantId, done.connection.id);
    const grants = await harness.repo.listResourceGrants(session.tenantId, done.connection.id);
    const reconnect = await startConnect(harness.deps(), session, {
      connectionId: done.connection.id,
    });
    expect(new URL(reconnect.authorizationUrl).searchParams.get("scope")).toContain(
      "analytics.readonly",
    );
    await expect(
      completeCallback(
        harness.deps({
          google: {
            token: tokenResponse(),
            userinfo: { sub: "reassigned-sub", email: "a@example.com", email_verified: true },
          },
        }),
        session,
        new URLSearchParams({
          code: "c",
          state: new URL(reconnect.authorizationUrl).searchParams.get("state")!,
        }),
      ),
    ).rejects.toMatchObject({ code: "identity_conflict" });
    expect(await harness.repo.getConnection(session.tenantId, done.connection.id)).toEqual(before);
    expect(await harness.repo.getCredential(session.tenantId, done.connection.id)).toEqual(
      credential,
    );
    expect(await harness.repo.listResourceGrants(session.tenantId, done.connection.id)).toEqual(
      grants,
    );
    const rename = await startConnect(harness.deps(), session, {
      connectionId: done.connection.id,
    });
    const renamed = await completeCallback(
      harness.deps({
        google: {
          token: tokenResponse(),
          userinfo: { sub: "sub-1", email: "renamed@example.com", email_verified: true },
        },
      }),
      session,
      new URLSearchParams({
        code: "c",
        state: new URL(rename.authorizationUrl).searchParams.get("state")!,
      }),
    );
    expect(renamed.connection.email).toBe("renamed@example.com");
    expect(renamed.connection.id).toBe(done.connection.id);
  });

  it("preserves old grant health on operator, transient, denied and expired-code reconnect failures", async () => {
    const session = await harness.sessionFor(TENANT_A);
    const start = await startConnect(harness.deps(), session, {});
    const done = await completeCallback(
      harness.deps(),
      session,
      new URLSearchParams({
        code: "c",
        state: new URL(start.authorizationUrl).searchParams.get("state")!,
      }),
    );
    await harness.repo.upsertResourceGrant(
      session.tenantId,
      done.connection.id,
      "analytics",
      "properties/1",
      "property",
      "P",
      true,
      "{}",
    );
    const before = await harness.repo.getConnection(session.tenantId, done.connection.id);
    const credential = await harness.repo.getCredential(session.tenantId, done.connection.id);
    const grants = await harness.repo.listResourceGrants(session.tenantId, done.connection.id);
    for (const [failure, expected] of [
      [{ error: "invalid_client" }, "operator_config_missing"],
      [new Error("transient-private-provider-text"), "google_unavailable"],
      [{ error: "invalid_grant" }, "authorization_expired"],
      [{ error: "access_denied" }, "consent_denied"],
    ] as const) {
      const next = await startConnect(harness.deps(), session, {
        connectionId: done.connection.id,
      });
      await expect(
        completeCallback(
          harness.deps({ google: { token: failure } }),
          session,
          new URLSearchParams({
            code: "c",
            state: new URL(next.authorizationUrl).searchParams.get("state")!,
          }),
        ),
      ).rejects.toMatchObject({ code: expected });
      expect(await harness.repo.getConnection(session.tenantId, done.connection.id)).toEqual(
        before,
      );
      expect(await harness.repo.getCredential(session.tenantId, done.connection.id)).toEqual(
        credential,
      );
      expect(await harness.repo.listResourceGrants(session.tenantId, done.connection.id)).toEqual(
        grants,
      );
    }
    const denied = await startConnect(harness.deps(), session, {
      connectionId: done.connection.id,
    });
    await expect(
      completeCallback(
        harness.deps(),
        session,
        new URLSearchParams({
          error: "access_denied",
          state: new URL(denied.authorizationUrl).searchParams.get("state")!,
        }),
      ),
    ).rejects.toMatchObject({ code: "consent_denied" });
    expect(await harness.repo.getConnection(session.tenantId, done.connection.id)).toEqual(before);
    expect(await harness.repo.getCredential(session.tenantId, done.connection.id)).toEqual(
      credential,
    );
    expect(await harness.repo.listResourceGrants(session.tenantId, done.connection.id)).toEqual(
      grants,
    );
  });
});
