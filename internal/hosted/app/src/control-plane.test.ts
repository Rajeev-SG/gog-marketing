/**
 * Focused #64 control-plane contracts against real native D1.
 *
 * These tests deliberately use development-only fetch mocks for Google and the
 * private runner. They are not final hosted-UI or real Google acceptance.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import type { Database, ResourceGrant } from "../../state/src/types.js";
import { CredentialCipher, parseCredentialKeys } from "./credential-crypto.js";
import {
  discoverConnection,
  saveResourceGrants,
  type ConnectDeps,
  type ConnectSession,
} from "./connect.js";
import { GoogleOAuthClient } from "./google-oauth.js";
import type { DiscoveryRunner, DiscoveryOutcome } from "./discovery.js";
import { renderHome } from "./html.js";
import worker from "./index.js";
import type { Env } from "./index.js";
import {
  buildCookieRequest,
  generateTestKey,
  signTestToken,
  type TestKeyPair,
} from "./test-helpers.js";

const ORIGIN = "https://test.example.com";
const CANONICAL_ORIGIN = "https://gog-marketing.rajeev-sgill.workers.dev";
const CLERK_PUBLISHABLE_KEY = "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA";
const CLERK_ISSUER = "https://oriented-wombat-2813.clerk.accounts.dev";
const CREDENTIAL_KEY = Buffer.alloc(32, 5).toString("base64");
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

let mf: Miniflare | undefined;
let keys: TestKeyPair;

beforeEach(async () => {
  keys = await generateTestKey();
});

afterEach(async () => {
  await mf?.dispose();
  mf = undefined;
});

async function createEnv(overrides: Partial<Env> = {}): Promise<Env> {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "hosted-control-plane-test",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "control-plane-test" },
          script: "export default {};",
          modules: true,
        },
      ],
    }),
  );
  const db = (await mf.getD1Database("DB")) as unknown as D1Database;
  for (const file of ["0001_initial_schema.sql", "0002_oauth_states_discovery.sql"]) {
    for (const statement of splitMigrationStatements(loadMigrationSql(file))) {
      const result = await db.batch([db.prepare(statement)]);
      if (!result[0]?.success) throw new Error(`migration failed: ${statement}`);
    }
  }
  const baseEnv: Env = {
    DB: db,
    CLERK_SECRET_KEY: "sk_test_dummy_for_tests",
    CLERK_PUBLISHABLE_KEY,
    CLERK_ISSUER,
    CLERK_AUTHORIZED_PARTIES: ORIGIN,
    CLERK_JWT_KEY: keys.publicPem,
  };
  return { ...baseEnv, ...overrides };
}

async function authenticatedRequest(
  url: string,
  sub: string,
  env: Env,
  options: { method?: string; body?: string } = {},
): Promise<Response> {
  const token = await signTestToken({ sub, azp: ORIGIN, iss: CLERK_ISSUER }, keys.privateJwk);
  const request = new Request(url, {
    method: options.method ?? "GET",
    headers: { Authorization: `Bearer ${token}`, Origin: ORIGIN },
    body: options.body,
  });
  if (options.body) request.headers.set("Content-Type", "application/json");
  return worker.fetch(request, env);
}

async function cookieRequest(
  url: string,
  sub: string,
  env: Env,
  body: string,
  origin?: string,
): Promise<Response> {
  const token = await signTestToken({ sub, azp: ORIGIN, iss: CLERK_ISSUER }, keys.privateJwk);
  const request = new Request(url, {
    method: "POST",
    headers: {
      Cookie: `__session=${token}; __client_uat=1; __clerk_db_jwt=test_dev_browser`,
      "Content-Type": "application/json",
      ...(origin ? { Origin: origin } : {}),
    },
    body,
  });
  return worker.fetch(request, env);
}

async function tenantId(env: Env, clerkUser: string): Promise<string> {
  const repo = new HostedRepository(env.DB as unknown as Database);
  const tenant = await repo.bootstrapTenant(clerkUser);
  return tenant.id;
}

async function pkcs8Base64(): Promise<string> {
  const pair = (await crypto.subtle.generateKey(
    {
      name: "RSASSA-PKCS1-v1_5",
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256",
    },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const raw = await crypto.subtle.exportKey("pkcs8", pair.privateKey);
  return Buffer.from(raw).toString("base64");
}

async function readRepo(env: Env): Promise<HostedRepository> {
  return new HostedRepository(env.DB as unknown as Database);
}

describe("control plane route and UI contracts (#64)", () => {
  it("rejects protected resource and mutation routes without authentication", async () => {
    const env = await createEnv();
    for (const [path, init] of [
      ["/api/google/resources", { method: "GET" }],
      [
        "/api/google/grants",
        { method: "POST", body: "{}", headers: { "Content-Type": "application/json" } },
      ],
      [
        "/api/google/connections/not-real/discover",
        { method: "POST", body: "{}", headers: { "Content-Type": "application/json" } },
      ],
    ] as const) {
      const response = await worker.fetch(new Request(ORIGIN + path, init), env);
      expect(response.status).toBe(401);
      expect(((await response.json()) as { error: string }).error).toBe("Authentication required.");
    }
  });

  it("shows the safe missing-origin panel without deriving MCP authority from request hints", async () => {
    const env = await createEnv();
    const request = new Request(ORIGIN + "/", {
      headers: new Headers(await sessionHeaders(env, "user_panel_missing")),
    });
    request.headers.set("Host", CANONICAL_ORIGIN.replace("https://", ""));
    const response = await worker.fetch(request, env);
    expect(response.status).toBe(200);
    const html = await response.text();
    expect(html).toContain("Workspace address is not configured.");
    expect(html).toContain("authentication is not enabled yet");
    expect(html).not.toContain(`${CANONICAL_ORIGIN}/mcp`);
    expect(html).toContain('<button id="copy-mcp-url" type="button" disabled>');
  });

  it("exposes the canonical MCP URL from trusted operator configuration and no internal identifiers", async () => {
    const env = await createEnv({
      GOG_HOSTED_CANONICAL_ORIGIN: CANONICAL_ORIGIN,
    });
    const response = await authenticatedRequest(ORIGIN + "/", "user_panel", env);
    expect(response.status).toBe(200);
    const html = await response.text();
    expect(html).toContain(`${CANONICAL_ORIGIN}/mcp`);
    expect(html).toContain("When authentication is enabled, add the URL as a remote MCP server");
    expect(html).not.toContain("evil.example.com");
    expect(html).not.toMatch(UUID_RE);
  });

  it("requires trusted same-origin JSON for protected grant mutations", async () => {
    const env = await createEnv();
    const clerkUser = "user_csrf64";
    await authenticatedRequest(`${ORIGIN}/api/tenant`, clerkUser, env);
    const repo = await readRepo(env);
    const tenantIdValue = await tenantId(env, clerkUser);
    const connection = await repo.createConnection(
      tenantIdValue,
      "sub_csrf",
      "csrf@example.com",
      "",
      "[]",
    );
    await repo.upsertResourceGrant(
      tenantIdValue,
      connection.id,
      "analytics",
      "properties/1",
      "property",
      "Before",
      false,
      "{}",
    );
    const body = JSON.stringify({
      connectionId: connection.id,
      grants: [{ service: "analytics", resourceId: "properties/1", enabled: true }],
    });
    const noOrigin = await cookieRequest(ORIGIN + "/api/google/grants", clerkUser, env, body);
    expect(noOrigin.status).toBe(403);
    const foreignOrigin = await cookieRequest(
      ORIGIN + "/api/google/grants",
      clerkUser,
      env,
      body,
      "https://evil-origin.example.com",
    );
    expect(foreignOrigin.status).toBe(403);
    const grants = await repo.listResourceGrants(tenantIdValue, connection.id);
    expect(grants[0]?.enabled).toBe(false);
  });

  it("lists only owned persisted assets, saves a valid batch, and preserves defaults", async () => {
    const env = await createEnv();
    const userA = "user_assets_a";
    const userB = "user_assets_b";
    await authenticatedRequest(`${ORIGIN}/api/tenant`, userA, env);
    await authenticatedRequest(`${ORIGIN}/api/tenant`, userB, env);
    const repo = await readRepo(env);
    const tenantA = await tenantId(env, userA);
    const tenantB = await tenantId(env, userB);
    const connectionA = await repo.createConnection(
      tenantA,
      "sub_a64",
      "assets-a@example.com",
      "",
      "[]",
    );
    const connectionB = await repo.createConnection(
      tenantB,
      "sub_b64",
      "assets-b@example.com",
      "",
      "[]",
    );
    await repo.upsertResourceGrant(
      tenantA,
      connectionA.id,
      "analytics",
      "properties/enabled-one",
      "property",
      "Enabled",
      true,
      "{}",
    );
    await repo.upsertResourceGrant(
      tenantA,
      connectionA.id,
      "analytics",
      "properties/disabled-one",
      "property",
      "Disabled",
      false,
      "{}",
    );
    await repo.upsertResourceGrant(
      tenantB,
      connectionB.id,
      "analytics",
      "properties/foreign-one",
      "property",
      "Foreign",
      false,
      "{}",
    );

    const list = await authenticatedRequest(ORIGIN + "/api/google/resources", userA, env);
    expect(list.status).toBe(200);
    const listBody = (await list.json()) as {
      connections: Array<{
        id: string;
        resources: Array<{ service: string; resourceId: string; enabled: boolean }>;
      }>;
    };
    expect(listBody.connections).toHaveLength(1);
    expect(listBody.connections[0]?.id).toBe(connectionA.id);
    expect(listBody.connections[0]?.resources.map((resource) => resource.enabled)).toEqual([
      false,
      true,
    ]);
    expect(JSON.stringify(listBody)).not.toContain(tenantA);
    expect(JSON.stringify(listBody)).not.toContain(tenantB);

    const before = await repo.listResourceGrants(tenantB, connectionB.id);
    const foreign = await authenticatedRequest(ORIGIN + "/api/google/grants", userA, env, {
      method: "POST",
      body: JSON.stringify({
        connectionId: connectionB.id,
        grants: [{ service: "analytics", resourceId: "properties/foreign-one", enabled: true }],
      }),
    });
    expect(foreign.status).toBe(404);
    expect(await repo.listResourceGrants(tenantB, connectionB.id)).toEqual(before);

    const mixed = await authenticatedRequest(ORIGIN + "/api/google/grants", userA, env, {
      method: "POST",
      body: JSON.stringify({
        connectionId: connectionA.id,
        grants: [
          { service: "analytics", resourceId: "properties/disabled-one", enabled: true },
          { service: "analytics", resourceId: "properties/unknown", enabled: false },
        ],
      }),
    });
    expect(mixed.status).toBe(400);
    const unchanged = await repo.listResourceGrants(tenantA, connectionA.id);
    expect(unchanged.map((grant) => grant.enabled)).toEqual([false, true]);

    const save = await authenticatedRequest(ORIGIN + "/api/google/grants", userA, env, {
      method: "POST",
      body: JSON.stringify({
        connectionId: connectionA.id,
        tenantId: tenantB,
        grants: [{ service: "analytics", resourceId: "properties/disabled-one", enabled: true }],
      }),
    });
    expect(save.status).toBe(200);
    const saved = await repo.listResourceGrants(tenantA, connectionA.id);
    expect(saved.map((grant: ResourceGrant) => grant.enabled)).toEqual([true, true]);
  });

  it("preserves grants and choices across discovery success, failure, empty, and refresh", async () => {
    const runnerCalls: Array<Record<string, unknown>> = [];
    let discoveryResult: { resources: unknown[]; statuses: Record<string, unknown> } = {
      resources: [],
      statuses: {},
    };
    const stub = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "https://oauth2.googleapis.com/token") {
        return Response.json({
          access_token: "ya29.private-access",
          refresh_token: "1//private-refresh",
          token_type: "Bearer",
          expires_in: 3600,
          scope: "openid email https://www.googleapis.com/auth/analytics.readonly",
        });
      }
      if (url === "https://openidconnect.googleapis.com/v1/userinfo") {
        return Response.json({
          sub: "google-sub-64",
          email: "discovery@example.com",
          email_verified: true,
          name: "Discovery",
        });
      }
      if (url === "https://sts.googleapis.com/v1/token") {
        return Response.json({ access_token: "federated-access" });
      }
      if (url.includes(":generateIdToken")) {
        return Response.json({ token: `native-header.${nativeTokenPayload}.native-signature` });
      }
      if (url === `${ORIGIN}/v1/execute` || url === "https://runner.example/v1/execute") {
        if (init?.body)
          runnerCalls.push(JSON.parse(new TextDecoder().decode(init.body as Uint8Array)));
        return Response.json({
          ok: true,
          operation: "discover",
          result: discoveryResult,
        });
      }
      throw new Error(`unexpected fetch: ${url}`);
    }) as typeof fetch;

    const env = await createEnv({
      GOG_GOOGLE_OAUTH_CLIENT_ID:
        "629716276051-cq3jnl899hj4ie3f3vhokke8aebc4ff8.apps.googleusercontent.com",
      GOG_GOOGLE_OAUTH_CLIENT_SECRET: "test-secret",
      GOG_GOOGLE_OAUTH_REDIRECT_URI: `${ORIGIN}/oauth/google/callback`,
      GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: CREDENTIAL_KEY,
      GOG_CLOUD_RUN_SERVICE_URL: "https://runner.example/v1/execute",
      GOG_RUNNER_WIF_PROVIDER:
        "projects/629716276051/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
      GOG_RUNNER_WIF_ISSUER: ORIGIN,
      GOG_RUNNER_WIF_KEY_ID: "test-kid",
      GOG_RUNNER_WIF_SIGNING_KEY: await pkcs8Base64(),
      GOG_RUNNER_SERVICE_ACCOUNT: "runner@example.iam.gserviceaccount.com",
      GOG_RUNNER_INVOCATION_TOKEN: "invocation-secret-at-least-32-bytes",
      __testFetch: stub,
    });
    const nativeTokenExpiry = Math.floor((Date.now() + 300_000) / 1000);
    const nativeTokenPayload = Buffer.from(JSON.stringify({ exp: nativeTokenExpiry })).toString(
      "base64url",
    );
    // The stub's token is returned below; its payload must be valid because
    // the runner client parses it before sending the private request.
    const user = "user_discovery64";
    const start = await authenticatedRequest(ORIGIN + "/api/google/connect", user, env, {
      method: "POST",
      body: JSON.stringify({ services: ["analytics"] }),
    });
    expect(start.status).toBe(200);
    const startBody = (await start.json()) as { authorizationUrl: string; connectionId: string };
    const repo = await readRepo(env);
    const tenant = await tenantId(env, user);
    await repo.upsertResourceGrant(
      tenant,
      startBody.connectionId,
      "analytics",
      "properties/kept-enabled",
      "property",
      "Kept",
      true,
      "{}",
    );
    discoveryResult = {
      resources: [
        {
          service: "analytics",
          resource_id: "properties/kept-enabled",
          resource_type: "property",
          display_name: "Kept",
          enabled: true,
          parent: "parents/example",
          metadata: { parent: "parents/example" },
        },
        ...Array.from({ length: 5 }, (_, index) => ({
          service: "analytics",
          resource_id: `properties/new-${index + 1}`,
          resource_type: "property",
          display_name: `New ${index + 1}`,
          enabled: false,
          metadata: {},
        })),
      ],
      statuses: {
        analytics: {
          state: "ok",
          detail: "",
          resource_count: 6,
          checked_at: "2026-10-06T00:00:00Z",
        },
      },
    };
    const callbackUrl = `${ORIGIN}/oauth/google/callback?code=code64&state=${encodeURIComponent(new URL(startBody.authorizationUrl).searchParams.get("state")!)}`;
    const callback = await authenticatedRequest(callbackUrl, user, env);
    expect(callback.status).toBe(200);
    const callbackBody = (await callback.json()) as { discovery: { status: string } };
    expect(callbackBody.discovery.status).toBe("ok");
    let grants = await repo.listResourceGrants(tenant, startBody.connectionId);
    expect(grants).toHaveLength(6);
    expect(grants.find((grant) => grant.resourceId === "properties/kept-enabled")?.enabled).toBe(
      true,
    );
    expect(
      grants
        .filter((grant) => grant.resourceId !== "properties/kept-enabled")
        .every((grant) => !grant.enabled),
    ).toBe(true);

    discoveryResult = {
      resources: grants.map((grant) => ({
        service: grant.service,
        resource_id: grant.resourceId,
        resource_type: grant.resourceType,
        display_name: grant.displayName,
        enabled: grant.enabled,
        metadata: {},
      })),
      statuses: {
        analytics: {
          state: "error",
          detail: "google_api_error",
          resource_count: 6,
          checked_at: "2026-10-06T00:01:00Z",
        },
      },
    };
    const failed = await authenticatedRequest(
      `${ORIGIN}/api/google/connections/${startBody.connectionId}/discover`,
      user,
      env,
      { method: "POST", body: "{}" },
    );
    expect(failed.status).toBe(200);
    expect(((await failed.json()) as { discovery: { status: string } }).discovery.status).toBe(
      "error",
    );
    const afterFailure = await repo.listResourceGrants(tenant, startBody.connectionId);
    expect(afterFailure).toEqual(grants);

    discoveryResult = {
      resources: [],
      statuses: {
        analytics: {
          state: "ok",
          detail: "",
          resource_count: 0,
          checked_at: "2026-10-06T00:02:00Z",
        },
      },
    };
    const empty = await authenticatedRequest(
      `${ORIGIN}/api/google/connections/${startBody.connectionId}/discover`,
      user,
      env,
      { method: "POST", body: "{}" },
    );
    expect(empty.status).toBe(200);
    expect(((await empty.json()) as { discovery: { status: string } }).discovery.status).toBe(
      "empty",
    );
    const afterEmpty = await repo.listResourceGrants(tenant, startBody.connectionId);
    expect(afterEmpty).toEqual(grants);

    const refresh = await authenticatedRequest(
      `${ORIGIN}/api/google/connections/${startBody.connectionId}/refresh`,
      user,
      env,
      { method: "POST", body: "{}" },
    );
    expect(refresh.status).toBe(200);
    const afterRefresh = await repo.listResourceGrants(tenant, startBody.connectionId);
    expect(afterRefresh).toEqual(grants);

    const resources = await authenticatedRequest(ORIGIN + "/api/google/resources", user, env);
    const payload = JSON.stringify(await resources.json());
    expect(payload).toContain("properties/kept-enabled");
    expect(payload).not.toContain("ya29.private-access");
    expect(payload).not.toContain("1//private-refresh");
    expect(payload).not.toContain(tenant);
    expect(runnerCalls.every((call) => call.operation === "discover")).toBe(true);
  });

  it("keeps hosted reconnect, failed reload, and destructive disconnect contracts", async () => {
    const html = renderHome({
      publishableKey: CLERK_PUBLISHABLE_KEY,
      authorizedParties: [ORIGIN],
      clerkDomain: "oriented-wombat-2813.clerk.accounts.dev",
      mcpOrigin: CANONICAL_ORIGIN,
    });
    const scriptStart = html.lastIndexOf("<script>") + "<script>".length;
    const scriptEnd = html.indexOf("</script>", scriptStart);
    const script = html.slice(scriptStart, scriptEnd);
    const extraction = (name: string): string => {
      const start = script.indexOf(`function ${name}(`);
      if (start < 0) throw new Error(`missing UI function: ${name}`);
      const marker = "\n    }";
      const end = script.indexOf(marker, start);
      if (end < 0) throw new Error(`missing UI function end: ${name}`);
      return script.slice(start, end + marker.length);
    };

    const reconnect = new Function(
      "startConnection",
      "selectedServices",
      "connection",
      `${extraction("reconnect")}; return reconnect(connection);`,
    );
    const reconnectRequest: Record<string, unknown> = {};
    const connection = { id: "owned-connection", email: "owner@example.com" };
    (reconnect as (start: unknown, selected: () => string[], context: unknown) => void)(
      (request: Record<string, unknown>) => Object.assign(reconnectRequest, request),
      () => ["analytics", "tagmanager"],
      connection,
    );
    expect(reconnectRequest.connectionId).toBe("owned-connection");
    expect(reconnectRequest.services).toEqual(["analytics", "tagmanager"]);

    const guarded = new Function(
      "message",
      "request",
      "loadResources",
      "connection",
      `${extraction("guardedAction").replace("function guardedAction(", "async function guardedAction(")}; return guardedAction('Refresh', connection, request);`,
    );
    const messages: Array<string | undefined> = [];
    await (
      guarded as (
        message: (text?: string) => void,
        request: () => Promise<{ ok: true }>,
        load: () => Promise<boolean>,
      ) => Promise<void>
    )(
      (text = "") => messages.push(text),
      () => Promise.resolve({ ok: true }),
      () => Promise.resolve(false),
    );
    expect(messages).toEqual([""]);

    const disconnect = new Function(
      "guardedAction",
      "window",
      "jsonFetch",
      "connection",
      `${extraction("disconnect")}; return disconnect(connection);`,
    );
    const mutations: Array<{ label: string; request: () => Promise<unknown> }> = [];
    const guardedAction = (label: string, target: unknown, request: () => Promise<unknown>) =>
      mutations.push({ label, request });
    (disconnect as unknown as (...args: unknown[]) => void)(
      guardedAction,
      { confirm: () => false },
      () => Promise.resolve(new Response()),
      connection,
    );
    expect(mutations).toHaveLength(0);
    (disconnect as unknown as (...args: unknown[]) => void)(
      guardedAction,
      { confirm: () => true },
      () => Promise.resolve(new Response()),
      connection,
    );
    expect(mutations).toHaveLength(1);
    expect(mutations[0]!.label).toBe("Disconnect");
    expect(String(html)).toContain(
      "connections/' + encodeURIComponent(connection.id) + '/disconnect",
    );
  });

  it("preserves a concurrent disable when discovery resumes from its stale snapshot", async () => {
    const env = await createEnv();
    const repository = await readRepo(env);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const oauth = new GoogleOAuthClient({
      clientId: "test-client-id",
      clientSecret: "test-client-secret",
    });
    const session: ConnectSession = { userId: "race-user", sessionId: "session", tenantId: "" };
    const tenant = await tenantId(env, session.userId);
    session.tenantId = tenant;
    const connection = await repository.createConnection(
      tenant,
      "race-google-subject",
      "race@example.com",
      "",
      JSON.stringify(["https://www.googleapis.com/auth/analytics.readonly"]),
    );
    const encrypted = await cipher.encrypt(tenant, connection.id, {
      access_token: "ya29.race-access",
      refresh_token: "1//race-refresh",
      token_type: "Bearer",
      expiry: new Date(Date.now() + 300_000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/analytics.readonly"],
    });
    await repository.upsertCredential({
      tenantId: tenant,
      connectionId: connection.id,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    await repository.upsertResourceGrant(
      tenant,
      connection.id,
      "analytics",
      "properties/race",
      "property",
      "Race",
      true,
      "{}",
    );

    let paused = false;
    let release: (() => void) | undefined;
    const gate = new Promise<void>((resolve) => (release = resolve));
    const discovery: DiscoveryRunner = {
      async discover(): Promise<DiscoveryOutcome> {
        return {
          status: "ok",
          detail: "",
          resources: [
            {
              service: "analytics",
              resourceId: "properties/race",
              resourceType: "property",
              displayName: "Race",
              parent: "",
              metadata: {},
            },
          ],
          statuses: {},
        };
      },
    };
    const snapshotRepo = new Proxy(repository, {
      get(source, property) {
        if (property === "listResourceGrants") {
          return async (...args: unknown[]) => {
            const result = await source.listResourceGrants(args[0] as string, args[1] as string);
            paused = true;
            await gate;
            return result;
          };
        }
        const value = Reflect.get(source, property, source);
        return typeof value === "function" ? value.bind(source) : value;
      },
    });
    const deps: ConnectDeps = {
      repo: snapshotRepo as HostedRepository,
      oauth,
      cipher,
      discovery,
      redirectUri: `${ORIGIN}/oauth/google/callback`,
      stateTtlSeconds: 600,
    };
    const discoveryPromise = discoverConnection(deps, session, connection.id);
    for (let spins = 0; spins < 200 && !paused; spins += 1) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    expect(paused).toBe(true);
    await saveResourceGrants(repository, session, connection.id, {
      grants: [{ service: "analytics", resourceId: "properties/race", enabled: false }],
    });
    release?.();
    await discoveryPromise;
    const grants = await repository.listResourceGrants(tenant, connection.id);
    expect(grants.find((grant) => grant.resourceId === "properties/race")?.enabled).toBe(false);
  });

  it("persists 251 discovered resources without overwriting existing choices", async () => {
    const env = await createEnv();
    const repository = await readRepo(env);
    const tenant = await tenantId(env, "user-251");
    const connection = await repository.createConnection(
      tenant,
      "subject-251",
      "inventory@example.com",
      "",
      "[]",
    );
    await repository.upsertResourceGrant(
      tenant,
      connection.id,
      "analytics",
      "properties/choice",
      "property",
      "Existing choice",
      true,
      "{}",
    );
    const resources = [
      {
        service: "analytics",
        resourceId: "properties/choice",
        resourceType: "property",
        displayName: "Updated existing",
        enabled: true,
        metadataJson: "{}",
      },
      ...Array.from({ length: 250 }, (_, index) => ({
        service: "analytics",
        resourceId: `properties/inventory-${index + 1}`,
        resourceType: "property",
        displayName: `Inventory ${index + 1}`,
        enabled: false,
        metadataJson: "{}",
      })),
    ];
    await repository.upsertResourceGrants(tenant, connection.id, resources);
    const grants = await repository.listResourceGrants(tenant, connection.id);
    expect(grants).toHaveLength(251);
    expect(grants.find((grant) => grant.resourceId === "properties/choice")).toMatchObject({
      enabled: true,
      displayName: "Updated existing",
    });
    expect(
      grants
        .filter((grant) => grant.resourceId !== "properties/choice")
        .every((grant) => !grant.enabled),
    ).toBe(true);
  });

  it("native-D1 regression 101: invalid last metadata leaves zero rows changed and preserves the existing choice", async () => {
    const env = await createEnv();
    const repository = await readRepo(env);
    const tenant = await tenantId(env, "user-101-regression");
    const connection = await repository.createConnection(
      tenant,
      "subject-101-regression",
      "regression101@example.com",
      "",
      "[]",
    );
    await repository.upsertResourceGrant(
      tenant,
      connection.id,
      "analytics",
      "properties/choice",
      "property",
      "Existing choice",
      true,
      "{}",
    );
    const resources = [
      {
        service: "analytics",
        resourceId: "properties/choice",
        resourceType: "property",
        displayName: "Updated existing",
        enabled: true,
        metadataJson: "{}",
      },
      ...Array.from({ length: 99 }, (_, index) => ({
        service: "analytics",
        resourceId: `properties/inventory-${index + 1}`,
        resourceType: "property",
        displayName: `Inventory ${index + 1}`,
        enabled: false,
        metadataJson: "{}",
      })),
      {
        service: "analytics",
        resourceId: "properties/overflow",
        resourceType: "property",
        displayName: "Oversized metadata",
        enabled: false,
        // Valid JSON whose string exceeds the 16,384-character limit, so the
        // rejection must come from the metadata length check, not JSON parsing.
        metadataJson: JSON.stringify({ data: "x".repeat(16_384) }),
      },
    ];
    await expect(repository.upsertResourceGrants(tenant, connection.id, resources)).rejects.toThrow(
      "metadataJson",
    );
    const grants = await repository.listResourceGrants(tenant, connection.id);
    expect(grants).toHaveLength(1);
    expect(grants[0]).toMatchObject({
      resourceId: "properties/choice",
      enabled: true,
      displayName: "Existing choice",
      metadataJson: "{}",
    });
  });

  it("rejects colon-tuple collisions before any grant write and audits a valid save", async () => {
    const env = await createEnv();
    const user = "user_grant_collision64";
    await authenticatedRequest(`${ORIGIN}/api/tenant`, user, env);
    const repo = await readRepo(env);
    const tenantIdValue = await tenantId(env, user);
    const connection = await repo.createConnection(
      tenantIdValue,
      "subject-collision64",
      "collision@example.com",
      "",
      "[]",
    );
    await repo.upsertResourceGrant(
      tenantIdValue,
      connection.id,
      "searchconsole",
      "https://example.test/",
      "site",
      "Exact site",
      false,
      "{}",
    );
    await repo.upsertResourceGrant(
      tenantIdValue,
      connection.id,
      "analytics",
      "properties/valid",
      "property",
      "Valid",
      false,
      "{}",
    );
    const before = await repo.listResourceGrants(tenantIdValue, connection.id);

    const collision = await authenticatedRequest(ORIGIN + "/api/google/grants", user, env, {
      method: "POST",
      body: JSON.stringify({
        connectionId: connection.id,
        grants: [
          { service: "analytics", resourceId: "properties/valid", enabled: true },
          { service: "searchconsole", resourceId: "https://example.test/", enabled: true },
          { service: "searchconsole:https", resourceId: "//example.test/", enabled: false },
        ],
      }),
    });
    expect(collision.status).toBe(400);
    expect(await repo.listResourceGrants(tenantIdValue, connection.id)).toEqual(before);
    expect(await repo.listAudit(tenantIdValue, 10)).toHaveLength(0);

    const save = await authenticatedRequest(ORIGIN + "/api/google/grants", user, env, {
      method: "POST",
      body: JSON.stringify({
        connectionId: connection.id,
        grants: [{ service: "analytics", resourceId: "properties/valid", enabled: true }],
      }),
    });
    expect(save.status).toBe(200);
    const grants = await repo.listResourceGrants(tenantIdValue, connection.id);
    expect(grants.find((grant) => grant.resourceId === "properties/valid")?.enabled).toBe(true);
    expect(grants.find((grant) => grant.service === "searchconsole")?.enabled).toBe(false);

    const audits = await repo.listAudit(tenantIdValue, 10);
    const savedAudit = audits.find((event) => event.action === "google.grants.save");
    expect(savedAudit?.connectionId).toBe(connection.id);
    expect(savedAudit?.actorClerkUserId).toBe(user);
    expect(savedAudit?.result).toBe("allow");
    expect(JSON.parse(savedAudit?.detailJson ?? "{}")).toEqual({
      operation: "save",
      count: 1,
      error: "none",
    });
    const responseText = JSON.stringify(await save.json());
    expect(responseText).not.toContain(tenantIdValue);
    expect(responseText).not.toContain("ya29");
    expect(JSON.stringify(audits)).not.toContain("https://example.test/");
  });
});

async function sessionHeaders(env: Env, sub: string): Promise<Headers> {
  const token = await signTestToken({ sub, azp: ORIGIN, iss: CLERK_ISSUER }, keys.privateJwk);
  const response = await worker.fetch(buildCookieRequest(`${ORIGIN}/api/tenant`, token, true), env);
  expect(response.status).toBe(200);
  const headers = new Headers();
  headers.set("Authorization", `Bearer ${token}`);
  return headers;
}
