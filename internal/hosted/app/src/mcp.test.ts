/**
 * Remote MCP Streamable HTTP contracts (#65).
 *
 * Uses real Clerk JWT verification with a generated RSA key and native D1.
 * No request field is allowed to select tenant, connection, or resource
 * authority.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import type { Database } from "../../state/src/types.js";
import {
  CredentialCipher,
  parseCredentialKeys,
  type StoredGoogleCredential,
} from "./credential-crypto.js";
import worker from "./index.js";
import type { Env } from "./index.js";
import { generateTestKey, signTestToken, type TestKeyPair } from "./test-helpers.js";
import type { ToolExecutionRequest, ToolRunner } from "./tool-runner.js";

const RESOURCE = "https://mcp.example.test/mcp";
const ORIGIN = "https://mcp.example.test";
const CLERK_PUBLISHABLE_KEY = "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA";
const CLERK_ISSUER = "https://oriented-wombat-2813.clerk.accounts.dev";
const CREDENTIAL_KEY = Buffer.alloc(32, 7).toString("base64");
const ACCESS_TOKEN = "ya29.hosted-tool-secret-access-token";
const REFRESH_TOKEN = "1//hosted-tool-secret-refresh-token";

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
          name: "mcp-test",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "mcp-test" },
          script: "export default {};",
          modules: true,
        },
      ],
    }),
  );
  const db = (await mf.getD1Database("DB")) as unknown as D1Database;
  for (const file of [
    "0001_initial_schema.sql",
    "0002_oauth_states_discovery.sql",
    "0003_rate_limit_counters.sql",
    "0004_mcp_signal_counters.sql",
    "0005_tool_grants.sql",
  ]) {
    for (const statement of splitMigrationStatements(loadMigrationSql(file))) {
      const result = await db.batch([db.prepare(statement)]);
      if (!result[0]?.success) throw new Error(`migration failed: ${statement}`);
    }
  }
  return {
    DB: db,
    CLERK_SECRET_KEY: "sk_test_dummy_for_tests",
    CLERK_PUBLISHABLE_KEY,
    CLERK_ISSUER,
    CLERK_AUTHORIZED_PARTIES: ORIGIN,
    CLERK_JWT_KEY: keys.publicPem,
    GOG_HOSTED_CANONICAL_ORIGIN: ORIGIN,
    GOG_GOOGLE_OAUTH_CLIENT_ID: "test-client-id",
    GOG_GOOGLE_OAUTH_CLIENT_SECRET: "test-client-secret",
    GOG_GOOGLE_OAUTH_REDIRECT_URI: `${ORIGIN}/oauth/google/callback`,
    GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: CREDENTIAL_KEY,
    ...overrides,
  };
}

async function tokenFor(
  sub: string,
  options: {
    aud?: string;
    exp?: number;
    iss?: string;
    typ?: "at+jwt" | "application/at+jwt";
  } = {},
): Promise<string> {
  return signTestToken(
    {
      sub,
      azp: "codex-client",
      typ: options.typ ?? "at+jwt",
      aud: options.aud ?? RESOURCE,
      exp: options.exp,
      iss: options.iss ?? CLERK_ISSUER,
    },
    keys.privateJwk,
  );
}

async function rpc(
  env: Env,
  method: string,
  params?: unknown,
  options: {
    token?: string | null;
    id?: string | number | null;
    body?: string;
    ip?: string;
  } = {},
): Promise<Response> {
  const id = options.id === undefined ? "req-1" : options.id;
  const body =
    options.body ??
    JSON.stringify({
      jsonrpc: "2.0",
      ...(id === null ? {} : { id }),
      method,
      ...(params === undefined ? {} : { params }),
    });
  return worker.fetch(
    new Request(RESOURCE, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json, text/event-stream",
        ...(options.ip ? { "cf-connecting-ip": options.ip } : {}),
        ...(options.token === null
          ? {}
          : { Authorization: `Bearer ${options.token ?? (await tokenFor("user_rpc"))}` }),
      },
      body,
    }),
    env,
  );
}

async function rpcJson(response: Response): Promise<any> {
  const text = await response.text();
  const data = text
    .split("\n")
    .filter((line) => line.startsWith("data: "))
    .map((line) => JSON.parse(line.slice(6)));
  return data.at(-1) ?? JSON.parse(text);
}

async function grantService(
  env: Env,
  user: string,
  service: string,
  enabled = true,
): Promise<void> {
  const repo = new HostedRepository(env.DB as unknown as Database);
  const tenant = await repo.bootstrapTenant(user);
  const connection = await repo.createConnection(
    tenant.id,
    `sub-${user}-${service}`,
    `${user}-${service}@example.test`,
    "",
    "[]",
  );
  await repo.upsertResourceGrant(
    tenant.id,
    connection.id,
    service,
    `resource-${service}`,
    service === "analytics" ? "property" : "resource",
    service,
    enabled,
    "{}",
  );
  if (service === "analytics")
    await repo.saveToolGrants(
      tenant.id,
      connection.id,
      ["analytics_properties_get", "analytics_report", "analytics_datastreams_list"].map(
        (toolName) => ({
          service,
          toolName,
          enabled: true,
          allowDestructive: false,
          allowBillable: false,
        }),
      ),
    );
}

interface ToolFixture {
  repo: HostedRepository;
  tenantId: string;
  connectionId: string;
  resourceId: string;
}

async function createToolFixture(
  env: Env,
  user: string,
  enabled = true,
  resourceId = "properties/123",
): Promise<ToolFixture> {
  const repo = new HostedRepository(env.DB as unknown as Database);
  const tenant = await repo.bootstrapTenant(user);
  const connection = await repo.createConnection(
    tenant.id,
    `google-${user}`,
    `${user}@google.example.test`,
    "Test account",
    "[]",
  );
  await repo.upsertResourceGrant(
    tenant.id,
    connection.id,
    "analytics",
    resourceId,
    "property",
    "Test property",
    enabled,
    "{}",
  );
  await repo.saveToolGrants(
    tenant.id,
    connection.id,
    ["analytics_properties_get", "analytics_report", "analytics_datastreams_list"].map(
      (toolName) => ({
        service: "analytics",
        toolName,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      }),
    ),
  );
  const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
  const credential: StoredGoogleCredential = {
    access_token: ACCESS_TOKEN,
    refresh_token: REFRESH_TOKEN,
    token_type: "Bearer",
    expiry: new Date(Date.now() + 3_600_000).toISOString(),
    granted_scopes: ["openid", "https://www.googleapis.com/auth/analytics.readonly"],
  };
  const encrypted = await cipher.encrypt(tenant.id, connection.id, credential);
  await repo.upsertCredential({
    tenantId: tenant.id,
    connectionId: connection.id,
    ciphertext: encrypted.ciphertext,
    nonce: encrypted.nonce,
    keyVersion: Number(encrypted.keyVersion),
  });
  return { repo, tenantId: tenant.id, connectionId: connection.id, resourceId };
}

function mockToolRunner(): {
  runner: ToolRunner;
  requests: ToolExecutionRequest[];
} {
  const requests: ToolExecutionRequest[] = [];
  const runner: ToolRunner = {
    async execute(request) {
      requests.push(request);
      return {
        status: "ok",
        result: {
          name: request.resourceId,
          displayName: "Test property",
          timeZone: "Europe/London",
          currencyCode: "GBP",
        },
      };
    },
  };
  return { runner, requests };
}

describe("remote MCP discovery and dynamic registration (#65)", () => {
  it("publishes protected-resource metadata and proxies Clerk authorization metadata", async () => {
    const metadata = {
      issuer: CLERK_ISSUER,
      authorization_endpoint: `${CLERK_ISSUER}/oauth/authorize`,
      token_endpoint: `${CLERK_ISSUER}/oauth/token`,
      jwks_uri: `${CLERK_ISSUER}/.well-known/jwks.json`,
      code_challenge_methods_supported: ["S256"],
    };
    const env = await createEnv({
      __testFetch: (async (input: RequestInfo | URL) => {
        expect(String(input)).toBe(`${CLERK_ISSUER}/.well-known/openid-configuration`);
        return Response.json(metadata);
      }) as typeof fetch,
    });

    const protectedResource = await worker.fetch(
      new Request(`${ORIGIN}/.well-known/oauth-protected-resource`),
      env,
    );
    expect(protectedResource.status).toBe(200);
    expect(await protectedResource.json()).toMatchObject({
      resource: RESOURCE,
      authorization_servers: [CLERK_ISSUER],
      bearer_methods_supported: ["header"],
    });

    const authorizationServer = await worker.fetch(
      new Request(`${ORIGIN}/.well-known/oauth-authorization-server`),
      env,
    );
    expect(authorizationServer.status).toBe(200);
    expect(await authorizationServer.json()).toMatchObject({
      ...metadata,
      registration_endpoint: `${ORIGIN}/oauth/register`,
    });
  });

  it("returns a structured registration error when Clerk DCR is not advertised", async () => {
    const env = await createEnv({
      __testFetch: (async () =>
        Response.json({
          issuer: CLERK_ISSUER,
          authorization_endpoint: `${CLERK_ISSUER}/oauth/authorize`,
        })) as typeof fetch,
    });
    const response = await worker.fetch(
      new Request(`${ORIGIN}/oauth/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          client_name: "Codex",
          redirect_uris: ["https://client.example/cb"],
        }),
      }),
      env,
    );
    expect(response.status).toBe(501);
    expect(await response.json()).toMatchObject({
      error: "registration_unavailable",
      error_description: expect.stringContaining("Manually register a public OAuth client"),
    });
  });

  it("proxies DCR only to Clerk's same-origin advertised registration endpoint", async () => {
    const calls: string[] = [];
    const env = await createEnv({
      __testFetch: (async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        calls.push(url);
        if (url.endsWith("/openid-configuration")) {
          return Response.json({
            issuer: CLERK_ISSUER,
            registration_endpoint: `${CLERK_ISSUER}/oauth/register`,
          });
        }
        expect(init?.method).toBe("POST");
        return Response.json({ client_id: "registered-client" }, { status: 201 });
      }) as typeof fetch,
    });
    const response = await worker.fetch(
      new Request(`${ORIGIN}/oauth/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ client_name: "Codex" }),
      }),
      env,
    );
    expect(response.status).toBe(201);
    expect(await response.json()).toEqual({ client_id: "registered-client" });
    expect(calls).toEqual([
      `${CLERK_ISSUER}/.well-known/openid-configuration`,
      `${CLERK_ISSUER}/oauth/register`,
    ]);
  });
});

describe("MCP Streamable HTTP and bearer authority (#65)", () => {
  it("rejects GET/SSE with 405 and serves stream-capable POST responses", async () => {
    const env = await createEnv();
    const get = await worker.fetch(
      new Request(RESOURCE, { headers: { Accept: "text/event-stream" } }),
      env,
    );
    expect(get.status).toBe(405);
    expect(((await get.json()) as { error: { code: number } }).error.code).toBe(-32600);

    const initialized = await rpc(env, "initialize", {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "Codex", version: "1" },
    });
    expect(initialized.status).toBe(200);
    expect(initialized.headers.get("content-type")).toContain("text/event-stream");
    expect(await rpcJson(initialized)).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      result: {
        protocolVersion: "2025-11-25",
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: "gog-marketing" },
      },
    });
    expect(await rpcJson(await rpc(env, "ping"))).toEqual({
      jsonrpc: "2.0",
      id: "req-1",
      result: {},
    });
    const notification = await rpc(env, "notifications/initialized", undefined, { id: null });
    expect(notification.status).toBe(202);
    expect(await notification.text()).toBe("");
  });

  it("accepts Clerk OAuth JWT access-token types and rejects invalid bearer tokens structurally", async () => {
    const env = await createEnv();
    const missing = await rpc(env, "ping", undefined, { token: null });
    expect(missing.status).toBe(401);
    expect(missing.headers.get("www-authenticate")).toContain(
      `resource_metadata="${ORIGIN}/.well-known/oauth-protected-resource"`,
    );
    const missingBody = await missing.json();
    expect(missingBody).toMatchObject({
      jsonrpc: "2.0",
      error: { code: -32001, data: { reason: "invalid_request" } },
    });
    expect(
      (missingBody as { error: { data: Record<string, unknown> } }).error.data,
    ).not.toHaveProperty("retryAfterSeconds");

    for (const typ of ["at+jwt", "application/at+jwt"] as const) {
      const response = await rpc(env, "ping", undefined, {
        token: await tokenFor(`user_${typ}`, { typ }),
      });
      expect(response.status).toBe(200);
      expect(await rpcJson(response)).toMatchObject({
        jsonrpc: "2.0",
        result: {},
      });
    }

    for (const token of [
      await tokenFor("expired", { exp: Math.floor(Date.now() / 1000) - 60 }),
      await tokenFor("wrong-issuer", { iss: "https://other.example.test" }),
      await tokenFor("wrong-audience", { aud: "https://other-resource.example.test" }),
    ]) {
      const response = await rpc(env, "ping", undefined, { token });
      expect(response.status).toBe(401);
      expect(await response.json()).toMatchObject({
        jsonrpc: "2.0",
        error: { code: -32001, data: { reason: "invalid_token" } },
      });
    }

    const opaque = await rpc(env, "ping", undefined, { token: "oat_opaque_access_token" });
    expect(opaque.status).toBe(401);
    expect(opaque.headers.get("www-authenticate")).toContain('error="invalid_token"');
    expect(await opaque.json()).toMatchObject({
      jsonrpc: "2.0",
      error: { code: -32001, data: { reason: "opaque_token_unsupported" } },
    });
  });

  it("resolves tenant only from the verified bearer subject and filters tools by D1 grants", async () => {
    const env = await createEnv();
    await grantService(env, "user_a", "analytics", true);
    await grantService(env, "user_a", "googleads", false);
    await grantService(env, "user_b", "tagmanager", true);

    const tokenA = await tokenFor("user_a");
    const tokenB = await tokenFor("user_b");
    const listA = await rpcJson(
      await rpc(
        env,
        "tools/list",
        { tenantId: "user_b", connectionId: "foreign", resourceId: "foreign" },
        { token: tokenA },
      ),
    );
    const namesA = listA.result.tools.map((tool: { name: string }) => tool.name);
    expect(namesA).toEqual([
      "analytics_report",
      "analytics_properties_get",
      "analytics_datastreams_list",
    ]);
    expect(namesA).not.toContain("googleads_query");
    expect(namesA).not.toContain("tagmanager_accounts_list");
    expect(JSON.stringify(listA)).not.toContain("user_a");

    const listB = await rpcJson(await rpc(env, "tools/list", undefined, { token: tokenB }));
    const namesB = listB.result.tools.map((tool: { name: string }) => tool.name);
    expect(namesB).toEqual([]);
    expect(namesB).not.toContain("analytics_report");
  });

  it("fails closed and audits the full tool-call denial matrix before provider calls", async () => {
    const mock = mockToolRunner();
    let providerCalls = 0;
    const env = await createEnv({
      __testToolRunner: mock.runner,
      __testFetch: (async () => {
        providerCalls += 1;
        throw new Error("unexpected provider call");
      }) as typeof fetch,
    });
    const fixture = await createToolFixture(env, "user_denials", false);
    const foreignFixture = await createToolFixture(
      env,
      "user_foreign_resource_owner",
      true,
      "properties/456",
    );
    const token = await tokenFor("user_denials");
    const cases = [
      {
        detail: "unknown_tool",
        params: {
          name: "not_a_tool",
          arguments: {
            access_token: ACCESS_TOKEN,
            refresh_token: REFRESH_TOKEN,
            client_secret: "test-client-secret",
          },
        },
      },
      {
        detail: "resource_mapping_unknown",
        params: {
          name: "analytics_properties_get",
          arguments: { resourceId: fixture.resourceId },
        },
      },
      {
        detail: "grant_denied",
        params: {
          name: "analytics_accounts",
          arguments: {},
        },
      },
      {
        detail: "grant_denied",
        params: {
          name: "analytics_properties_get",
          arguments: { property: fixture.resourceId },
        },
      },
      {
        detail: "grant_denied",
        params: {
          name: "analytics_properties_get",
          arguments: { property: "properties/999" },
        },
      },
      {
        detail: "foreign_tenant",
        params: {
          name: "analytics_properties_get",
          arguments: { property: fixture.resourceId, tenantId: foreignFixture.tenantId },
        },
      },
      {
        detail: "connection_not_found",
        params: {
          name: "analytics_properties_get",
          arguments: {
            property: fixture.resourceId,
            connectionId: crypto.randomUUID(),
          },
        },
      },
      {
        detail: "connection_not_found",
        params: {
          name: "analytics_properties_get",
          arguments: {
            property: fixture.resourceId,
            connectionId: foreignFixture.connectionId,
          },
        },
      },
      {
        detail: "grant_denied",
        params: {
          name: "analytics_properties_get",
          arguments: { property: foreignFixture.resourceId },
        },
      },
    ];

    const bodies: unknown[] = [];
    for (const testCase of cases) {
      const response = await rpc(env, "tools/call", testCase.params, { token });
      const body = await rpcJson(response);
      bodies.push(body);
      expect(body).toMatchObject({
        jsonrpc: "2.0",
        id: "req-1",
        error: {
          code: -32602,
          data: { reason: "policy_denied", detail: testCase.detail },
        },
      });
    }

    expect(providerCalls).toBe(0);
    expect(mock.requests).toHaveLength(0);
    const audit = await fixture.repo.listAudit(fixture.tenantId, 20);
    expect(audit).toHaveLength(cases.length);
    expect(audit.every((event) => event.action === "mcp.tools.call")).toBe(true);
    expect(audit.every((event) => event.result === "deny")).toBe(true);
    for (const event of audit) {
      expect(event.latencyMs).toEqual(expect.any(Number));
      expect(Number.isInteger(event.latencyMs)).toBe(true);
    }
    const serialized = JSON.stringify({ bodies, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(REFRESH_TOKEN);
    expect(serialized).not.toContain("test-client-secret");
  });

  it("denies inherited property tool names and audits each attempt", async () => {
    const mock = mockToolRunner();
    let providerCalls = 0;
    const env = await createEnv({
      __testToolRunner: mock.runner,
      __testFetch: (async () => {
        providerCalls += 1;
        throw new Error("unexpected provider call");
      }) as typeof fetch,
    });
    const fixture = await createToolFixture(env, "user_unknown_names", false);
    const token = await tokenFor("user_unknown_names");

    for (const name of ["__proto__", "constructor", "toString"]) {
      const body = await rpcJson(await rpc(env, "tools/call", { name }, { token }));
      expect(body).toMatchObject({
        jsonrpc: "2.0",
        id: "req-1",
        error: {
          code: -32602,
          data: { reason: "policy_denied", detail: "unknown_tool" },
        },
      });
    }

    expect(providerCalls).toBe(0);
    expect(mock.requests).toHaveLength(0);
    const audit = await fixture.repo.listAudit(fixture.tenantId, 10);
    expect(audit).toHaveLength(3);
    expect(audit.every((event) => event.action === "mcp.tools.call")).toBe(true);
    expect(audit.every((event) => event.result === "deny")).toBe(true);
    for (const event of audit) {
      expect(JSON.parse(event.detailJson)).toEqual({
        operation: "unknown",
        service: "",
        resourceType: "",
        resource: "unknown",
        error: "permission_denied",
      });
      expect(event.latencyMs).toEqual(expect.any(Number));
      expect(Number.isInteger(event.latencyMs)).toBe(true);
    }
  });

  it("denies oversized resource ids with an audit record", async () => {
    const mock = mockToolRunner();
    const env = await createEnv({ __testToolRunner: mock.runner });
    const fixture = await createToolFixture(env, "user_oversized_resource", true);
    const resourceId = `properties/${"9".repeat(256)}`;

    const body = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_properties_get", arguments: { property: resourceId } },
        { token: await tokenFor("user_oversized_resource") },
      ),
    );
    expect(body).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      error: {
        code: -32602,
        data: { reason: "policy_denied", detail: "resource_mapping_unknown" },
      },
    });
    expect(mock.requests).toHaveLength(0);
    const audit = await fixture.repo.listAudit(fixture.tenantId, 10);
    expect(audit).toHaveLength(1);
    expect(audit[0]).toMatchObject({ result: "deny" });
    expect(JSON.parse(audit[0]!.detailJson)).toEqual({
      operation: "analytics_properties_get",
      service: "analytics",
      resourceType: "property",
      resource: "unknown",
      error: "invalid_request",
    });
  });

  // This multi-step native D1/JWT case competes with Go builds on the two-CPU
  // Oracle runner. Keep every assertion and a finite, test-only deadline.
  it(
    "routes the added GA4 reads with encrypted scope authority and rejects disabled properties",
    { timeout: 20_000 },
    async () => {
      const requests: ToolExecutionRequest[] = [];
      const env = await createEnv({
        __testToolRunner: {
          async execute(request) {
            requests.push(request);
            return { status: "ok", result: { verifiedFixture: true } };
          },
        },
      });
      const fixture = await createToolFixture(env, "user_native_reads", true);
      const token = await tokenFor("user_native_reads");
      for (const name of ["analytics_report", "analytics_datastreams_list"]) {
        const result = await rpcJson(
          await rpc(
            env,
            "tools/call",
            {
              name,
              arguments: { property: fixture.resourceId, connectionId: fixture.connectionId },
            },
            { token },
          ),
        );
        expect(result.result.structuredContent).toEqual({ verifiedFixture: true });
        expect(requests.at(-1)!.nativeTool).toEqual({
          name,
          arguments: { property: fixture.resourceId },
          scopes: ["openid", "https://www.googleapis.com/auth/analytics.readonly"],
          allowedTools: [name],
          allowWrite: false,
          allowDestructive: false,
        });
        expect(requests.at(-1)!.tenantId).toBe(fixture.tenantId);
      }
      await fixture.repo.setResourceGrantsEnabled(fixture.tenantId, fixture.connectionId, [
        { service: "analytics", resourceId: fixture.resourceId, enabled: false },
      ]);
      const denied = await rpcJson(
        await rpc(
          env,
          "tools/call",
          { name: "analytics_report", arguments: { property: fixture.resourceId } },
          { token },
        ),
      );
      expect(denied.error.data.detail).toBe("grant_denied");
      expect(requests).toHaveLength(2);
    },
  );

  it("routes Calendar event creation only for an enabled calendar and tool", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { event: { id: "created" } } };
        },
      },
    });
    const fixture = await createToolFixture(env, "calendar-owner", true);
    const calendar = "owner+beta@example.test";
    await fixture.repo.upsertResourceGrant(
      fixture.tenantId,
      fixture.connectionId,
      "calendar",
      calendar,
      "calendar",
      "Calendar",
      true,
      "{}",
    );
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/calendar"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("calendar-owner");
    const params = {
      name: "calendar_event_create",
      arguments: {
        calendar_id: calendar,
        summary: "Beta test",
        from: "2026-10-12T10:00:00Z",
        to: "2026-10-12T11:00:00Z",
      },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "calendar",
        toolName: params.name,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const created = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(created.result.structuredContent).toEqual({ event: { id: "created" } });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ resourceType: "calendar", resourceId: calendar });
    expect(requests[0]!.nativeTool).toMatchObject({
      name: params.name,
      arguments: params.arguments,
      allowWrite: true,
    });
    const foreign = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { ...params, arguments: { ...params.arguments, calendar_id: "foreign@example.test" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });

  it("routes account Contacts writes without fabricated contact assets", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { contact: { resourceName: "people/c7" } } };
        },
      },
    });
    const fixture = await createToolFixture(env, "contacts-owner", true);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/contacts"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("contacts-owner");
    const params = {
      name: "contacts_create",
      arguments: { given: "Hosted", email: "test@example.test" },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "contacts",
        toolName: params.name,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const created = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(created.result.structuredContent).toEqual({ contact: { resourceName: "people/c7" } });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      service: "contacts",
      resourceType: "",
      resourceId: "",
      connectionId: fixture.connectionId,
    });
    expect(requests[0]!.nativeTool).toMatchObject({
      arguments: params.arguments,
      allowWrite: true,
      scopes: ["https://www.googleapis.com/auth/contacts"],
    });
  });

  it("routes sitemap submission only for an enabled Search Console site", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { submitted: true } };
        },
      },
    });
    const fixture = await createToolFixture(env, "search-owner", true);
    const site = "https://example.test/";
    await fixture.repo.upsertResourceGrant(
      fixture.tenantId,
      fixture.connectionId,
      "searchconsole",
      site,
      "site",
      "Site",
      true,
      "{}",
    );
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/webmasters"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("search-owner");
    const params = {
      name: "searchconsole_sitemaps_submit",
      arguments: { site_url: site, feed_path: "https://example.test/sitemap.xml" },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "searchconsole",
        toolName: params.name,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const submitted = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(submitted.result.structuredContent).toEqual({ submitted: true });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ resourceType: "site", resourceId: site });
    expect(requests[0]!.nativeTool).toMatchObject({
      name: params.name,
      arguments: params.arguments,
      scopes: ["https://www.googleapis.com/auth/webmasters"],
      allowWrite: true,
    });
    const foreign = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { ...params, arguments: { ...params.arguments, site_url: "https://foreign.test/" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });

  it("routes explicit GA4 Admin writes only within an enabled property", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { name: "properties/123/keyEvents/7" } };
        },
      },
    });
    const fixture = await createToolFixture(env, "admin-owner", true);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/analytics.edit"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("admin-owner");
    const params = {
      name: "analytics_keyevents_create",
      arguments: { property: fixture.resourceId, body_json: '{"eventName":"hosted_test"}' },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "analytics",
        toolName: params.name,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const created = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(created.result.structuredContent).toEqual({ name: "properties/123/keyEvents/7" });
    expect(requests).toHaveLength(1);
    expect(requests[0]!.nativeTool).toMatchObject({
      name: params.name,
      arguments: params.arguments,
      scopes: ["https://www.googleapis.com/auth/analytics.edit"],
      allowWrite: true,
    });
    const foreign = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { ...params, arguments: { ...params.arguments, property: "properties/999" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });

  it("routes GTM publishing only for an enabled canonical container and tool", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { published: true } };
        },
      },
    });
    const fixture = await createToolFixture(env, "gtm-owner", true);
    const container = "accounts/111/containers/222";
    await fixture.repo.upsertResourceGrant(
      fixture.tenantId,
      fixture.connectionId,
      "tagmanager",
      container,
      "container",
      "GTM",
      true,
      "{}",
    );
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/tagmanager.publish"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("gtm-owner");
    const params = {
      name: "tagmanager_versions_publish",
      arguments: { container_path: container, version: "7" },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "tagmanager",
        toolName: "tagmanager_versions_publish",
        enabled: true,
        allowDestructive: true,
        allowBillable: false,
      },
    ]);
    const published = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(published.result.structuredContent).toEqual({ published: true });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      service: "tagmanager",
      resourceType: "container",
      resourceId: container,
    });
    expect(requests[0]!.nativeTool).toMatchObject({
      name: params.name,
      arguments: params.arguments,
      allowWrite: true,
      allowDestructive: true,
    });
    const foreign = await rpcJson(
      await rpc(
        env,
        "tools/call",
        {
          ...params,
          arguments: { ...params.arguments, container_path: "accounts/111/containers/333" },
        },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });

  it("routes account Tasks without fake resources and rejects a foreign account", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { id: "new-list" } };
        },
      },
    });
    const fixture = await createToolFixture(env, "tasks-owner", true);
    const foreign = await createToolFixture(env, "tasks-foreign", true);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/tasks"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("tasks-owner");
    const params = {
      name: "tasks_lists_create",
      arguments: { title: "New list" },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "tasks",
        toolName: "tasks_lists_create",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const created = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(created.result.structuredContent).toEqual({ id: "new-list" });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      connectionId: fixture.connectionId,
      service: "tasks",
      resourceType: "",
      resourceId: "",
    });
    expect(requests[0]!.nativeTool!.arguments).toEqual({ title: "New list" });
    const rejected = await rpcJson(
      await rpc(env, "tools/call", { ...params, connectionId: foreign.connectionId }, { token }),
    );
    expect(rejected.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);

    const sibling = await fixture.repo.createConnection(
      fixture.tenantId,
      "tasks-sibling",
      "sibling@example.test",
      "Sibling",
      "[]",
    );
    await fixture.repo.saveToolGrants(fixture.tenantId, sibling.id, [
      {
        service: "tasks",
        toolName: "tasks_lists_create",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const ambiguous = await rpcJson(
      await rpc(
        env,
        "tools/call",
        {
          name: params.name,
          arguments: params.arguments,
        },
        { token },
      ),
    );
    expect(ambiguous.error.data.detail).toBe("connection_ambiguous");
    expect(requests).toHaveLength(1);
    const listed = await rpcJson(await rpc(env, "tools/list", {}, { token }));
    const schema = listed.result.tools.find(
      (tool: { name: string }) => tool.name === params.name,
    ).inputSchema;
    expect(schema.additionalProperties).toBe(false);
    expect(schema.properties.connectionId.enum.sort()).toEqual(
      [fixture.connectionId, sibling.id].sort(),
    );
    expect(schema.properties.connectionId.enum).not.toContain(foreign.connectionId);
    expect(schema.required).toContain("connectionId");
    const siblingEncrypted = await cipher.encrypt(fixture.tenantId, sibling.id, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/tasks"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: sibling.id,
      ciphertext: siblingEncrypted.ciphertext,
      nonce: siblingEncrypted.nonce,
      keyVersion: Number(siblingEncrypted.keyVersion),
    });
    for (const selected of schema.properties.connectionId.enum) {
      const response = await rpcJson(
        await rpc(
          env,
          "tools/call",
          {
            name: params.name,
            arguments: { title: "New list", connectionId: selected },
          },
          { token },
        ),
      );
      expect(response.result.structuredContent).toEqual({ id: "new-list" });
      expect(requests.at(-1)!.connectionId).toBe(selected);
      expect(requests.at(-1)!.nativeTool!.arguments).toEqual({ title: "New list" });
    }
    expect(requests).toHaveLength(3);
  });

  it("routes an explicitly permitted selected-file write and denies after tool disable", async () => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { written: true } };
        },
      },
    });
    const fixture = await createToolFixture(env, "docs-owner", true);
    await fixture.repo.upsertResourceGrant(
      fixture.tenantId,
      fixture.connectionId,
      "docs",
      "selected-doc",
      "file",
      "Doc",
      true,
      "{}",
    );
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(fixture.tenantId, fixture.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/drive.file"],
    });
    await fixture.repo.upsertCredential({
      tenantId: fixture.tenantId,
      connectionId: fixture.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor("docs-owner");
    const params = {
      name: "docs_write",
      arguments: { document_id: "selected-doc", text: "hello" },
      connectionId: fixture.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "docs",
        toolName: "docs_write",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const written = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(written.result.structuredContent).toEqual({ written: true });
    expect(requests[0]!.nativeTool).toEqual({
      name: "docs_write",
      arguments: { document_id: "selected-doc", text: "hello" },
      scopes: ["https://www.googleapis.com/auth/drive.file"],
      allowedTools: ["docs_write"],
      allowWrite: true,
      allowDestructive: false,
    });
    await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
      {
        service: "docs",
        toolName: "docs_write",
        enabled: false,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const disabled = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });

  it("executes the mapped read through the runner and rechecks disabled grants", async () => {
    const mock = mockToolRunner();
    const env = await createEnv({ __testToolRunner: mock.runner });
    const fixture = await createToolFixture(env, "user_allow", true);
    const token = await tokenFor("user_allow");

    const first = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_properties_get", arguments: { property: fixture.resourceId } },
        { token },
      ),
    );
    expect(first).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      result: {
        content: [{ type: "text" }],
        structuredContent: {
          name: fixture.resourceId,
          displayName: "Test property",
          timeZone: "Europe/London",
          currencyCode: "GBP",
        },
      },
    });
    expect(mock.requests).toEqual([
      {
        tenantId: fixture.tenantId,
        connectionId: fixture.connectionId,
        service: "analytics",
        resourceType: "property",
        resourceId: fixture.resourceId,
        accessToken: ACCESS_TOKEN,
        googleEmail: "user_allow@google.example.test",
        googleSubject: "google-user_allow",
      },
    ]);

    await fixture.repo.setResourceEnabled(
      fixture.tenantId,
      fixture.connectionId,
      "analytics",
      fixture.resourceId,
      false,
    );
    const disabled = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_properties_get", arguments: { property: fixture.resourceId } },
        { token },
      ),
    );
    expect(disabled).toMatchObject({
      error: {
        code: -32602,
        data: { reason: "policy_denied", detail: "grant_denied" },
      },
    });
    expect(mock.requests).toHaveLength(1);

    await fixture.repo.setResourceEnabled(
      fixture.tenantId,
      fixture.connectionId,
      "analytics",
      fixture.resourceId,
      true,
    );
    const restored = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_properties_get", arguments: { property: fixture.resourceId } },
        { token },
      ),
    );
    expect(restored.result.structuredContent.name).toBe(fixture.resourceId);
    expect(mock.requests).toHaveLength(2);

    const audit = await fixture.repo.listAudit(fixture.tenantId, 20);
    expect(audit).toHaveLength(3);
    expect(audit.map((event) => event.result).sort()).toEqual(["allow", "allow", "deny"]);
    for (const event of audit) {
      expect(event).toMatchObject({
        tenantId: fixture.tenantId,
        actorClerkUserId: "user_allow",
        action: "mcp.tools.call",
      });
      expect(event.connectionId).toBe(event.result === "allow" ? fixture.connectionId : "");
      expect(event.latencyMs).toEqual(expect.any(Number));
      expect(Number.isInteger(event.latencyMs)).toBe(true);
    }
    const allowDetails = audit
      .filter((event) => event.result === "allow")
      .map((event) => JSON.parse(event.detailJson));
    expect(allowDetails).toEqual([
      {
        operation: "analytics_properties_get",
        service: "analytics",
        resourceType: "property",
        resource: fixture.resourceId,
      },
      {
        operation: "analytics_properties_get",
        service: "analytics",
        resourceType: "property",
        resource: fixture.resourceId,
      },
    ]);
    const serialized = JSON.stringify({ first, disabled, restored, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(REFRESH_TOKEN);
    expect(serialized).not.toContain("test-client-secret");
  });

  it("sanitizes runner failures and records only stable audit error codes", async () => {
    const runner: ToolRunner = {
      async execute() {
        throw new Error(`raw runner failure ${ACCESS_TOKEN} ${REFRESH_TOKEN}`);
      },
    };
    const env = await createEnv({ __testToolRunner: runner });
    const fixture = await createToolFixture(env, "user_runner_error", true);
    const response = await rpc(
      env,
      "tools/call",
      { name: "analytics_properties_get", arguments: { property: fixture.resourceId } },
      { token: await tokenFor("user_runner_error") },
    );
    const body = await rpcJson(response);
    expect(body).toMatchObject({
      error: { code: -32002, data: { reason: "execution_failed" } },
    });
    const audit = await fixture.repo.listAudit(fixture.tenantId, 10);
    expect(audit).toHaveLength(1);
    expect(audit[0]).toMatchObject({ result: "error", latencyMs: expect.any(Number) });
    expect(Number.isInteger(audit[0]!.latencyMs)).toBe(true);
    expect(JSON.parse(audit[0]!.detailJson)).toMatchObject({ error: "internal_error" });
    const serialized = JSON.stringify({ body, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(REFRESH_TOKEN);
    expect(serialized).not.toContain("raw runner failure");
  });

  it("preserves structured runner diagnostics in execution errors", async () => {
    const runner: ToolRunner = {
      async execute() {
        return {
          status: "error",
          detail: { runnerStatus: 502, runnerBodyJson: true },
        };
      },
    };
    const env = await createEnv({ __testToolRunner: runner });
    const fixture = await createToolFixture(env, "user_runner_diagnostics", true);
    const body = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_properties_get", arguments: { property: fixture.resourceId } },
        { token: await tokenFor("user_runner_diagnostics") },
      ),
    );

    expect(body).toMatchObject({
      error: {
        code: -32002,
        data: {
          reason: "execution_failed",
          runner_detail: { runnerStatus: 502, runnerBodyJson: true },
        },
      },
    });
    expect(JSON.stringify(body)).not.toContain("[object Object]");
  });

  it("returns JSON-RPC parse, invalid-request, and method-not-found errors", async () => {
    const now = Date.UTC(2026, 9, 8, 12, 0, 0);
    const env = await createEnv({ __testNow: () => now });
    const token = await tokenFor("user_errors");
    const parse = await rpc(env, "", undefined, { token, body: "{" });
    expect(parse.status).toBe(400);
    expect(((await parse.json()) as { error: { code: number } }).error.code).toBe(-32700);

    const invalid = await rpc(env, "ping", undefined, {
      token,
      body: JSON.stringify({ jsonrpc: "1.0", id: 1, method: "ping" }),
    });
    expect(invalid.status).toBe(400);
    expect(((await invalid.json()) as { error: { code: number } }).error.code).toBe(-32600);

    const emptyDocument = await rpc(env, "", undefined, {
      token,
      body: JSON.stringify({ jsonrpc: "2.0" }),
    });
    expect(emptyDocument.status).toBe(400);
    expect(await emptyDocument.json()).toMatchObject({
      jsonrpc: "2.0",
      id: null,
      error: { code: -32600 },
    });

    const missingMethod = await rpcJson(await rpc(env, "unknown/method", undefined, { token }));
    expect(missingMethod.error.code).toBe(-32601);

    const repo = new HostedRepository(env.DB as unknown as Database);
    expect(await repo.getMcpSignal("mcp_parse_error", "2026-10-08")).toMatchObject({ value: 1 });
    expect(await repo.getMcpSignal("mcp_invalid_request", "2026-10-08")).toMatchObject({
      value: 2,
    });
    expect(await repo.getMcpSignal("mcp_method_not_found", "2026-10-08")).toMatchObject({
      value: 1,
    });
  });

  it("returns a sanitized JSON-RPC internal error carrying the request id", async () => {
    const env = await createEnv();
    const db = env.DB;
    env.DB = new Proxy(db, {
      get(target, property, receiver) {
        if (property === "prepare") {
          return (sql: string) => {
            if (sql.includes("hosted_google_connections")) {
              throw new Error("forced D1 failure detail");
            }
            return target.prepare(sql);
          };
        }
        const value = Reflect.get(target, property, receiver);
        return typeof value === "function" ? value.bind(target) : value;
      },
    });

    const response = await rpc(env, "tools/list", undefined, {
      token: await tokenFor("user_internal_error"),
      id: "failed-op",
    });
    expect(response.status).toBe(500);
    expect(await response.json()).toEqual({
      jsonrpc: "2.0",
      id: "failed-op",
      error: { code: -32603, message: "Internal error." },
    });
  });
});

describe("MCP quota, abuse limits, and observability (#67)", () => {
  it("rate-limits repeated missing-bearer requests and records tenant-independent signals", async () => {
    const now = Date.UTC(2026, 9, 8, 12, 30, 0);
    const ip = "203.0.113.10";
    const env = await createEnv({
      GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "2",
      __testNow: () => now,
    });
    const repo = new HostedRepository(env.DB as unknown as Database);

    for (const expectedStatus of [401, 401]) {
      expect((await rpc(env, "ping", undefined, { token: null, ip })).status).toBe(expectedStatus);
    }
    const rejected = await rpc(env, "ping", undefined, { token: null, ip });
    const rejectedBody = await rpcJson(rejected);
    expect(rejected.status).toBe(429);
    expect(rejected.headers.get("www-authenticate")).toBeNull();
    expect(rejectedBody).toMatchObject({
      error: { code: -32029, data: { reason: "rate_limited", scope: "ip" } },
    });

    expect(await repo.getMcpSignal("auth_missing_bearer", "2026-10-08")).toMatchObject({
      value: 2,
    });
    expect(await repo.getMcpSignal("mcp_request_rate_limited", "2026-10-08")).toMatchObject({
      value: 1,
    });
    expect(
      JSON.stringify({
        body: rejectedBody,
        signals: [
          await repo.getMcpSignal("auth_missing_bearer", "2026-10-08"),
          await repo.getMcpSignal("mcp_request_rate_limited", "2026-10-08"),
        ],
      }),
    ).not.toContain(ip);
  });

  it("rate-limits repeated invalid-bearer requests and records tenant-independent signals", async () => {
    const now = Date.UTC(2026, 9, 8, 12, 31, 0);
    const ip = "203.0.113.11";
    const invalidBearer = "invalid-bearer-test-value";
    const env = await createEnv({
      GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "2",
      __testNow: () => now,
    });
    const repo = new HostedRepository(env.DB as unknown as Database);

    for (const expectedStatus of [401, 401]) {
      expect((await rpc(env, "ping", undefined, { token: invalidBearer, ip })).status).toBe(
        expectedStatus,
      );
    }
    const rejected = await rpc(env, "ping", undefined, { token: invalidBearer, ip });
    const rejectedBody = await rpcJson(rejected);
    expect(rejected.status).toBe(429);
    expect(rejectedBody).toMatchObject({
      error: { code: -32029, data: { reason: "rate_limited", scope: "ip" } },
    });

    expect(await repo.getMcpSignal("auth_invalid_bearer", "2026-10-08")).toMatchObject({
      value: 2,
    });
    expect(await repo.getMcpSignal("mcp_request_rate_limited", "2026-10-08")).toMatchObject({
      value: 1,
    });
    expect(
      JSON.stringify({
        body: rejectedBody,
        signals: [
          await repo.getMcpSignal("auth_invalid_bearer", "2026-10-08"),
          await repo.getMcpSignal("mcp_request_rate_limited", "2026-10-08"),
        ],
      }),
    ).not.toContain(ip);
    expect(
      JSON.stringify(await repo.getMcpSignal("auth_invalid_bearer", "2026-10-08")),
    ).not.toContain(invalidBearer);
  });

  it("allows the configured daily quota, denies exhaustion, and resets in the next UTC day", async () => {
    let now = Date.UTC(2026, 9, 8, 12, 0, 0);
    const mock = mockToolRunner();
    const env = await createEnv({
      GOG_MCP_TOOL_CALL_DAILY_LIMIT: "2",
      GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "100",
      __testNow: () => now,
      __testToolRunner: mock.runner,
    });
    const fixture = await createToolFixture(env, "user_quota", true);
    const token = await tokenFor("user_quota");
    const params = {
      name: "analytics_properties_get",
      arguments: {
        property: fixture.resourceId,
        access_token: ACCESS_TOKEN,
        refresh_token: REFRESH_TOKEN,
      },
    };

    const first = await rpc(env, "tools/call", params, { token });
    const second = await rpc(env, "tools/call", params, { token });
    expect(first.status).toBe(200);
    expect(second.status).toBe(200);
    expect((await rpcJson(first)).result.structuredContent.name).toBe(fixture.resourceId);
    expect((await rpcJson(second)).result.structuredContent.name).toBe(fixture.resourceId);

    const exhausted = await rpc(env, "tools/call", params, { token });
    const exhaustedBody = await rpcJson(exhausted);
    expect(exhausted.status).toBe(429);
    expect(exhausted.headers.get("www-authenticate")).toBeNull();
    expect(exhaustedBody).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      error: {
        code: -32003,
        message: "Daily tool-call quota exceeded.",
        data: {
          reason: "quota_exceeded",
          limit: 2,
          retryAfterSeconds: expect.any(Number),
        },
      },
    });
    expect(exhaustedBody.error.data.retryAfterSeconds).toBeGreaterThan(0);
    expect(mock.requests).toHaveLength(2);

    const firstDayQuota = await fixture.repo.getQuota(
      fixture.tenantId,
      "2026-10-08",
      "mcp_tool_calls",
    );
    expect(firstDayQuota?.value).toBe(2);

    now = Date.UTC(2026, 9, 9, 0, 0, 1);
    const reset = await rpc(env, "tools/call", params, { token });
    expect(reset.status).toBe(200);
    expect((await rpcJson(reset)).result.structuredContent.name).toBe(fixture.resourceId);
    const nextDayQuota = await fixture.repo.getQuota(
      fixture.tenantId,
      "2026-10-09",
      "mcp_tool_calls",
    );
    expect(nextDayQuota?.value).toBe(1);

    const audit = await fixture.repo.listAudit(fixture.tenantId, 10);
    expect(audit.map((event) => event.result).sort()).toEqual(["allow", "allow", "allow", "deny"]);
    expect(audit.find((event) => event.result === "deny")).toMatchObject({
      action: "mcp.tools.call",
    });
    expect(JSON.parse(audit.find((event) => event.result === "deny")!.detailJson)).toMatchObject({
      error: "quota_exceeded",
    });
    for (const event of audit) {
      expect(event.latencyMs).toEqual(expect.any(Number));
      expect(Number.isInteger(event.latencyMs)).toBe(true);
      expect(event.latencyMs!).toBeGreaterThanOrEqual(0);
    }

    const serialized = JSON.stringify({ exhaustedBody, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(REFRESH_TOKEN);
    expect(serialized).not.toContain(CREDENTIAL_KEY);
    expect(serialized).not.toContain("test-client-secret");
    expect(serialized).not.toContain(token);
  });

  it("rate-limits repeated tenant requests and records each 429 without secrets", async () => {
    const now = Date.UTC(2026, 9, 8, 12, 34, 0);
    const env = await createEnv({
      GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "2",
      __testNow: () => now,
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const tenant = await repo.bootstrapTenant("user_rate_tenant");
    const token = await tokenFor("user_rate_tenant");

    expect((await rpc(env, "ping", undefined, { token })).status).toBe(200);
    expect((await rpc(env, "ping", undefined, { token })).status).toBe(200);

    const rejectedBodies: unknown[] = [];
    for (let attempt = 0; attempt < 3; attempt++) {
      const response = await rpc(env, "ping", undefined, { token });
      const body = await rpcJson(response);
      rejectedBodies.push(body);
      expect(response.status).toBe(429);
      expect(response.headers.get("retry-after")).toMatch(/^\d+$/);
      expect(body).toMatchObject({
        jsonrpc: "2.0",
        error: {
          code: -32029,
          message: "Request rate limit exceeded.",
          data: {
            reason: "rate_limited",
            scope: "tenant",
            retryAfterSeconds: expect.any(Number),
          },
        },
      });
    }

    const audit = await repo.listAudit(tenant.id, 10);
    expect(audit).toHaveLength(3);
    expect(audit.every((event) => event.action === "mcp.request.rate_limited")).toBe(true);
    expect(audit.every((event) => event.result === "deny")).toBe(true);
    for (const event of audit) {
      expect(JSON.parse(event.detailJson)).toMatchObject({
        operation: "tenant",
        error: "rate_limited",
        count: expect.any(Number),
      });
    }

    const serialized = JSON.stringify({ rejectedBodies, audit });
    expect(serialized).not.toContain(token);
    expect(serialized).not.toContain(CREDENTIAL_KEY);
    expect(serialized).not.toContain("test-client-secret");
  });

  it("rate-limits a shared source IP across tenants without storing the raw IP", async () => {
    const now = Date.UTC(2026, 9, 8, 12, 35, 0);
    const ip = "203.0.113.42";
    const env = await createEnv({
      GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "2",
      __testNow: () => now,
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const tokenA = await tokenFor("user_ip_a");
    const tokenB = await tokenFor("user_ip_b");

    expect((await rpc(env, "ping", undefined, { token: tokenA, ip })).status).toBe(200);
    expect((await rpc(env, "ping", undefined, { token: tokenB, ip })).status).toBe(200);

    const rejected = await rpc(env, "ping", undefined, { token: tokenA, ip });
    const body = await rpcJson(rejected);
    expect(rejected.status).toBe(429);
    expect(body).toMatchObject({
      error: {
        code: -32029,
        data: { reason: "rate_limited", scope: "ip" },
      },
    });

    const signal = await repo.getMcpSignal("mcp_request_rate_limited", "2026-10-08");
    expect(signal).toMatchObject({ value: 1 });
    const serialized = JSON.stringify({ body, signal });
    expect(serialized).not.toContain(ip);
    expect(serialized).not.toContain(tokenA);
    expect(serialized).not.toContain(tokenB);
  });

  it("fails closed and audits when request rate counters error", async () => {
    const env = await createEnv({ GOG_MCP_REQUESTS_PER_MINUTE_LIMIT: "60" });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const tenant = await repo.bootstrapTenant("user_rate_failure");
    const token = await tokenFor("user_rate_failure");
    const db = env.DB;
    env.DB = new Proxy(db, {
      get(target, property, receiver) {
        if (property === "prepare") {
          return (sql: string) => {
            if (sql.includes("hosted_rate_limit_counters")) {
              throw new Error(`forced rate counter failure ${ACCESS_TOKEN}`);
            }
            return target.prepare(sql);
          };
        }
        const value = Reflect.get(target, property, receiver);
        return typeof value === "function" ? value.bind(target) : value;
      },
    });

    const response = await rpc(env, "ping", undefined, { token });
    const body = await rpcJson(response);
    expect(response.status).toBe(503);
    expect(body).toMatchObject({
      jsonrpc: "2.0",
      error: {
        code: -32603,
        message: "Request rate limiting is unavailable.",
        data: { reason: "rate_limiter_unavailable" },
      },
    });

    const audit = await repo.listAudit(tenant.id, 10);
    expect(audit).toHaveLength(1);
    expect(audit[0]).toMatchObject({ action: "mcp.request", result: "error" });
    expect(JSON.parse(audit[0]!.detailJson)).toMatchObject({ error: "internal_error" });
    const serialized = JSON.stringify({ body, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(token);
  });

  it("fails closed and audits when the tool-call quota counter errors", async () => {
    const mock = mockToolRunner();
    const env = await createEnv({ __testToolRunner: mock.runner });
    const fixture = await createToolFixture(env, "user_quota_failure", true);
    const token = await tokenFor("user_quota_failure");
    const db = env.DB;
    env.DB = new Proxy(db, {
      get(target, property, receiver) {
        if (property === "prepare") {
          return (sql: string) => {
            if (sql.includes("hosted_quota_counters")) {
              throw new Error(`forced quota counter failure ${REFRESH_TOKEN}`);
            }
            return target.prepare(sql);
          };
        }
        const value = Reflect.get(target, property, receiver);
        return typeof value === "function" ? value.bind(target) : value;
      },
    });

    const response = await rpc(
      env,
      "tools/call",
      {
        name: "analytics_properties_get",
        arguments: { property: fixture.resourceId, access_token: ACCESS_TOKEN },
      },
      { token },
    );
    const body = await rpcJson(response);
    expect(response.status).toBe(503);
    expect(body).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      error: {
        code: -32603,
        message: "Tool-call quota is unavailable.",
        data: { reason: "quota_unavailable" },
      },
    });
    expect(mock.requests).toHaveLength(0);

    const audit = await fixture.repo.listAudit(fixture.tenantId, 10);
    expect(audit).toHaveLength(1);
    expect(audit[0]).toMatchObject({
      action: "mcp.tools.call",
      result: "error",
      latencyMs: expect.any(Number),
    });
    expect(JSON.parse(audit[0]!.detailJson)).toMatchObject({ error: "internal_error" });
    const serialized = JSON.stringify({ body, audit });
    expect(serialized).not.toContain(ACCESS_TOKEN);
    expect(serialized).not.toContain(REFRESH_TOKEN);
    expect(serialized).not.toContain(token);
  });
});

// The in-product chat uses the very same MCP authorization/runner path, not an OAuth client.
describe("chat Google tools and durable write approvals", () => {
  async function chatFixture() {
    const { googleTools, GoogleApprovals } = await import("./chat/tools.js");
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { id: "fixture-result" } };
        },
      },
    });
    const { repo, tenantId, connectionId } = await createToolFixture(env, "chat-tools-owner");
    const tenant = (await repo.getTenant(tenantId))!;
    const session = { userId: "chat-tools-owner", sessionId: "session-owner", tenant };
    const values = new Map<string, unknown>();
    let tail: Promise<unknown> = Promise.resolve();
    const storage = {
      get: async (key: string) => structuredClone(values.get(key)),
      put: async (key: string, value: unknown) => {
        values.set(key, structuredClone(value));
      },
      list: async ({ prefix }: { prefix: string }) =>
        new Map([...values].filter(([key]) => key.startsWith(prefix))),
      delete: async (keys: string[]) => {
        for (const key of keys) values.delete(key);
      },
      transaction: (fn: (tx: unknown) => Promise<unknown>) => {
        const result = tail.then(() => fn(storage));
        tail = result.catch(() => {});
        return result;
      },
    } as unknown as DurableObjectStorage;
    const approvals = new GoogleApprovals(storage);
    let active = true;
    const authorize = async () => {
      if (!active) throw new Error("Session revoked");
    };
    await repo.saveToolGrants(tenantId, connectionId, [
      {
        service: "tasks",
        toolName: "tasks_lists_create",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(tenantId, connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: [
        "https://www.googleapis.com/auth/tasks",
        "https://www.googleapis.com/auth/analytics.readonly",
      ],
    });
    await repo.upsertCredential({
      tenantId,
      connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const tools = await googleTools(env, repo, session, approvals, authorize);
    const run = async (name: string, input: Record<string, unknown>, id = "call-1") =>
      tools[name]!.execute!(input, {
        toolCallId: id,
        messages: [],
        abortSignal: new AbortController().signal,
        context: undefined,
      });
    return {
      repo,
      tenantId,
      connectionId,
      session,
      approvals,
      values,
      tools,
      run,
      requests,
      revoke: () => {
        active = false;
      },
    };
  }

  it("lists only enabled account tools/resources and executes real shared-MCP reads", async () => {
    const f = await chatFixture();
    expect(f.tools).toHaveProperty("analytics_properties_get");
    expect(f.tools).not.toHaveProperty("gmail_send");
    const other = await f.repo.createConnection(
      f.tenantId,
      "second-google",
      "second@example.test",
      "Second account",
      "[]",
    );
    await f.repo.saveToolGrants(f.tenantId, other.id, [
      {
        service: "analytics",
        toolName: "analytics_properties_get",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    // A tool-grant alone on B cannot borrow A's selected resource availability.
    const access = (await f.run("google_access", {})) as { accounts: { connectionId: string }[] };
    expect(access.accounts.map((a) => a.connectionId)).toEqual([f.connectionId]);
    const read = await f.run("analytics_properties_get", {
      property: "properties/123",
      connectionId: f.connectionId,
    });
    expect(read).toHaveProperty("structuredContent");
    expect(f.requests).toHaveLength(1);
    expect(f.requests[0]!.tenantId).toBe(f.tenantId);
    expect(JSON.stringify(read)).not.toContain(ACCESS_TOKEN);
  });

  it("requires exact one-use approval, rejects changed arguments and concurrent replay", async () => {
    const f = await chatFixture();
    const input = { title: "Approved test list", connectionId: f.connectionId };
    await expect(f.run("tasks_lists_create", input)).rejects.toThrow("fresh approval");
    expect(await f.approvals.respond("forged", true, f.session)).toBe(false);
    await f.approvals.request("call-1", "tasks_lists_create", input, f.session);
    expect(await f.approvals.respond("call-1", true, { ...f.session, userId: "other-user" })).toBe(
      false,
    );
    expect(await f.approvals.respond("call-1", true, f.session)).toBe(true);
    expect(await f.approvals.respond("call-1", true, f.session)).toBe(true);
    expect(await f.approvals.respond("call-1", false, f.session)).toBe(false);
    await expect(
      f.run("tasks_lists_create", { ...input, title: "Changed by model" }),
    ).rejects.toThrow("fresh approval");
    const outcomes = await Promise.allSettled([
      f.run("tasks_lists_create", input),
      f.run("tasks_lists_create", input),
    ]);
    expect(outcomes.filter((r) => r.status === "fulfilled")).toHaveLength(1);
    expect(f.requests).toHaveLength(1);
    expect(f.requests[0]!.nativeTool).toMatchObject({
      name: "tasks_lists_create",
      allowWrite: true,
      arguments: { title: input.title },
    });
    expect(await f.approvals.respond("call-1", true, f.session)).toBe(false);
  });

  it("rechecks tool/resource permissions after approval and rejects foreign accounts", async () => {
    const f = await chatFixture();
    const input = { title: "Never create", connectionId: f.connectionId };
    await f.approvals.request("call-1", "tasks_lists_create", input, f.session);
    await f.approvals.respond("call-1", true, f.session);
    await f.repo.saveToolGrants(f.tenantId, f.connectionId, [
      {
        service: "tasks",
        toolName: "tasks_lists_create",
        enabled: false,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);
    expect(await f.run("tasks_lists_create", input)).toHaveProperty("isError", true);
    expect(
      await f.run("analytics_properties_get", {
        property: "properties/123",
        connectionId: "foreign-account",
      }),
    ).toHaveProperty("isError", true);
    await f.repo.upsertResourceGrant(
      f.tenantId,
      f.connectionId,
      "analytics",
      "properties/123",
      "property",
      "Property",
      false,
      "{}",
    );
    expect(
      await f.run("analytics_properties_get", {
        property: "properties/123",
        connectionId: f.connectionId,
      }),
    ).toHaveProperty("isError", true);
    expect(f.requests).toHaveLength(0);
  });

  it("rejects denied, expired and revoked-session approvals without a Google call", async () => {
    const f = await chatFixture();
    const input = { title: "Never create", connectionId: f.connectionId };
    await f.approvals.request("denied", "tasks_lists_create", input, f.session);
    await f.approvals.respond("denied", false, f.session);
    await expect(f.run("tasks_lists_create", input, "denied")).rejects.toThrow("fresh approval");
    await f.approvals.request("expired", "tasks_lists_create", input, f.session);
    const record = f.values.get("google-approval:expired") as { expiresAt: number };
    record.expiresAt = Date.now() - 1;
    f.values.set("google-approval:expired", record);
    expect(await f.approvals.respond("expired", true, f.session)).toBe(false);
    expect(await f.approvals.request("expired", "tasks_lists_create", input, f.session)).toBe(
      "denied",
    );
    await expect(f.run("tasks_lists_create", input, "expired")).rejects.toThrow("fresh approval");
    await f.approvals.request("revoked", "tasks_lists_create", input, f.session);
    await f.approvals.respond("revoked", true, f.session);
    f.revoke();
    await expect(f.run("tasks_lists_create", input, "revoked")).rejects.toThrow("Session revoked");
    expect(f.requests).toHaveLength(0);
  });
});

it("blank spreadsheet creation never seeds file authority and remains tenant/account bound", async () => {
  const requests: ToolExecutionRequest[] = [];
  const env = await createEnv({
    __testToolRunner: {
      async execute(request) {
        requests.push(request);
        return { status: "ok", result: { spreadsheetId: "new-ungranted-sheet" } };
      },
    },
  });
  const owner = await createToolFixture(env, "blank-sheet-owner", true);
  const foreign = await createToolFixture(env, "blank-sheet-foreign", true);
  const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
  const encrypted = await cipher.encrypt(owner.tenantId, owner.connectionId, {
    access_token: ACCESS_TOKEN,
    refresh_token: REFRESH_TOKEN,
    token_type: "Bearer",
    expiry: new Date(Date.now() + 3600000).toISOString(),
    granted_scopes: ["https://www.googleapis.com/auth/drive.file"],
  });
  await owner.repo.upsertCredential({
    tenantId: owner.tenantId,
    connectionId: owner.connectionId,
    ciphertext: encrypted.ciphertext,
    nonce: encrypted.nonce,
    keyVersion: Number(encrypted.keyVersion),
  });
  const token = await tokenFor("blank-sheet-owner");
  const params = {
    name: "sheets_create_blank",
    arguments: { title: "Blank fixture" },
    connectionId: owner.connectionId,
  };
  const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
  expect(denied.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(0);
  await owner.repo.saveToolGrants(
    owner.tenantId,
    owner.connectionId,
    ["sheets_create_blank", "sheets_metadata"].map((toolName) => ({
      service: "sheets",
      toolName,
      enabled: true,
      allowDestructive: false,
      allowBillable: false,
    })),
  );
  const grantsBefore = await owner.repo.listResourceGrants(owner.tenantId, owner.connectionId);
  const created = await rpcJson(await rpc(env, "tools/call", params, { token }));
  expect(created.result.structuredContent).toEqual({ spreadsheetId: "new-ungranted-sheet" });
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({
    connectionId: owner.connectionId,
    service: "sheets",
    resourceType: "",
    resourceId: "",
  });
  expect(await owner.repo.listResourceGrants(owner.tenantId, owner.connectionId)).toEqual(
    grantsBefore,
  );
  const read = await rpcJson(
    await rpc(
      env,
      "tools/call",
      {
        name: "sheets_metadata",
        arguments: { spreadsheet_id: "new-ungranted-sheet" },
        connectionId: owner.connectionId,
      },
      { token },
    ),
  );
  expect(read.error.data.detail).toBe("grant_denied");
  const guessed = await rpcJson(
    await rpc(env, "tools/call", { ...params, connectionId: foreign.connectionId }, { token }),
  );
  expect(guessed.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(1);
});

for (const entry of [
  { service: "docs", create: "docs_create_blank", read: "docs_get", selector: "document_id" },
  {
    service: "slides",
    create: "slides_create_blank",
    read: "slides_list_slides",
    selector: "presentation_id",
  },
  { service: "forms", create: "forms_create_blank", read: "forms_get", selector: "form_id" },
]) {
  it(`${entry.service} blank creation cannot seed file grants or cross a tenant`, async () => {
    const requests: ToolExecutionRequest[] = [];
    const createdId = `new-ungranted-${entry.service}`;
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { id: createdId } };
        },
      },
    });
    const user = `${entry.service}-blank-owner`;
    const owner = await createToolFixture(env, user, true);
    const foreign = await createToolFixture(env, `${entry.service}-blank-foreign`, true);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
    const encrypted = await cipher.encrypt(owner.tenantId, owner.connectionId, {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/drive.file"],
    });
    await owner.repo.upsertCredential({
      tenantId: owner.tenantId,
      connectionId: owner.connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    const token = await tokenFor(user);
    const params = {
      name: entry.create,
      arguments: { title: "Blank fixture" },
      connectionId: owner.connectionId,
    };
    const denied = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(denied.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);
    await owner.repo.saveToolGrants(
      owner.tenantId,
      owner.connectionId,
      [entry.create, entry.read].map((toolName) => ({
        service: entry.service,
        toolName,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      })),
    );
    const before = await owner.repo.listResourceGrants(owner.tenantId, owner.connectionId);
    const result = await rpcJson(await rpc(env, "tools/call", params, { token }));
    expect(result.result.structuredContent).toEqual({ id: createdId });
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      connectionId: owner.connectionId,
      service: entry.service,
      resourceType: "",
      resourceId: "",
    });
    expect(await owner.repo.listResourceGrants(owner.tenantId, owner.connectionId)).toEqual(before);
    const read = await rpcJson(
      await rpc(
        env,
        "tools/call",
        {
          name: entry.read,
          arguments: { [entry.selector]: createdId },
          connectionId: owner.connectionId,
        },
        { token },
      ),
    );
    expect(read.error.data.detail).toBe("grant_denied");
    const guessed = await rpcJson(
      await rpc(env, "tools/call", { ...params, connectionId: foreign.connectionId }, { token }),
    );
    expect(guessed.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(1);
  });
}

it("routes GA4 account inventory and gated property listing without granting returned properties", async () => {
  const requests: ToolExecutionRequest[] = [];
  const env = await createEnv({
    __testToolRunner: {
      async execute(request) {
        requests.push(request);
        if (request.nativeTool!.name === "analytics_accounts") {
          return {
            status: "ok",
            result: {
              accountSummaries: [
                { account: "accounts/111", propertySummaries: [{ property: "properties/777" }] },
              ],
            },
          };
        }
        return {
          status: "ok",
          result: { properties: [{ name: "properties/777", parent: "accounts/111" }] },
        };
      },
    },
  });
  const fixture = await createToolFixture(env, "analytics-inv-owner", true);
  const foreign = await createToolFixture(env, "analytics-inv-foreign", true);
  const token = await tokenFor("analytics-inv-owner");
  const accountsParams = {
    name: "analytics_accounts",
    arguments: {},
    connectionId: fixture.connectionId,
  };
  const propertiesParams = {
    name: "analytics_properties_list",
    arguments: { account: "accounts/111" },
    connectionId: fixture.connectionId,
  };

  const accountsDenied = await rpcJson(await rpc(env, "tools/call", accountsParams, { token }));
  expect(accountsDenied.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(0);

  await fixture.repo.saveToolGrants(fixture.tenantId, fixture.connectionId, [
    {
      service: "analytics",
      toolName: "analytics_accounts",
      enabled: true,
      allowDestructive: false,
      allowBillable: false,
    },
    {
      service: "analytics",
      toolName: "analytics_properties_list",
      enabled: true,
      allowDestructive: false,
      allowBillable: false,
    },
    {
      service: "analytics",
      toolName: "analytics_properties_get",
      enabled: true,
      allowDestructive: false,
      allowBillable: false,
    },
  ]);

  const accounts = await rpcJson(await rpc(env, "tools/call", accountsParams, { token }));
  expect(accounts.result.structuredContent).toEqual({
    accountSummaries: [
      { account: "accounts/111", propertySummaries: [{ property: "properties/777" }] },
    ],
  });
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({
    connectionId: fixture.connectionId,
    service: "analytics",
    resourceType: "",
    resourceId: "",
  });
  expect(requests[0]!.nativeTool!.name).toBe("analytics_accounts");
  expect(requests[0]!.nativeTool!.arguments).toEqual({});

  const propertiesDenied = await rpcJson(await rpc(env, "tools/call", propertiesParams, { token }));
  expect(propertiesDenied.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(1);

  await fixture.repo.upsertResourceGrant(
    fixture.tenantId,
    fixture.connectionId,
    "analytics",
    "accounts/111",
    "account",
    "Test account",
    true,
    "{}",
  );

  const properties = await rpcJson(await rpc(env, "tools/call", propertiesParams, { token }));
  expect(properties.result.structuredContent).toEqual({
    properties: [{ name: "properties/777", parent: "accounts/111" }],
  });
  expect(requests).toHaveLength(2);
  expect(requests[1]).toMatchObject({
    connectionId: fixture.connectionId,
    service: "analytics",
    resourceType: "account",
    resourceId: "accounts/111",
  });
  expect(requests[1]!.nativeTool!.name).toBe("analytics_properties_list");
  expect(requests[1]!.nativeTool!.arguments).toEqual({ account: "accounts/111" });

  const foreignAccount = await rpcJson(
    await rpc(
      env,
      "tools/call",
      { ...propertiesParams, arguments: { account: "accounts/222" } },
      { token },
    ),
  );
  expect(foreignAccount.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(2);

  const foreignConnection = await rpcJson(
    await rpc(
      env,
      "tools/call",
      { ...propertiesParams, connectionId: foreign.connectionId },
      { token },
    ),
  );
  expect(foreignConnection.error.data.detail).toBe("connection_not_found");
  expect(requests).toHaveLength(2);

  await fixture.repo.upsertResourceGrant(
    fixture.tenantId,
    fixture.connectionId,
    "analytics",
    "accounts/111",
    "account",
    "Test account",
    false,
    "{}",
  );
  const disabledResource = await rpcJson(await rpc(env, "tools/call", propertiesParams, { token }));
  expect(disabledResource.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(2);

  await fixture.repo.upsertResourceGrant(
    fixture.tenantId,
    fixture.connectionId,
    "analytics",
    "accounts/111",
    "account",
    "Test account",
    true,
    "{}",
  );
  const returnedProperty = await rpcJson(
    await rpc(
      env,
      "tools/call",
      {
        name: "analytics_properties_get",
        arguments: { property: "properties/777" },
        connectionId: fixture.connectionId,
      },
      { token },
    ),
  );
  expect(returnedProperty.error.data.detail).toBe("grant_denied");
  expect(requests).toHaveLength(2);
});

const docsReadToolNames = [
  "docs_header_list",
  "docs_footer_list",
  "docs_suggestions_list",
  "docs_info",
  "docs_list_tabs",
  "docs_structure",
  "docs_headings_list",
  "docs_paragraphs_list",
  "docs_tables_list",
  "docs_images_list",
  "docs_named_range_list",
];

it.each(docsReadToolNames)(
  "docs tool %s enforces per-file grants and tenant scoping",
  async (toolName) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { documentId: "selected-doc" } };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    // Owner tenant/connection (no file grant yet).
    const tenant = await repo.bootstrapTenant("user_docs_files");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_docs_files",
      "user_docs_files@google.example.test",
      "Test account",
      "[]",
    );
    const encryptFor = async (tId: string, cId: string): Promise<void> => {
      const credential: StoredGoogleCredential = {
        access_token: ACCESS_TOKEN,
        refresh_token: REFRESH_TOKEN,
        token_type: "Bearer",
        expiry: new Date(Date.now() + 3_600_000).toISOString(),
        granted_scopes: ["openid", "https://www.googleapis.com/auth/documents.readonly"],
      };
      const encrypted = await cipher.encrypt(tId, cId, credential);
      await repo.upsertCredential({
        tenantId: tId,
        connectionId: cId,
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: Number(encrypted.keyVersion),
      });
    };
    await encryptFor(tenant.id, connection.id);

    // Foreign tenant with its own connection and a granted file ("other-doc").
    const otherTenant = await repo.bootstrapTenant("user_docs_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_docs_other",
      "user_docs_other@google.example.test",
      "Other account",
      "[]",
    );
    await encryptFor(otherTenant.id, otherConnection.id);
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      "docs",
      "other-doc",
      "file",
      "Other doc",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      {
        service: "docs",
        toolName: "docs_info",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
      {
        service: "docs",
        toolName: "docs_list_tabs",
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);

    const token = await tokenFor("user_docs_files");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { document_id: "selected-doc", ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    // 1) Tool grant saved, but selected file has no grant yet: deny, zero dispatches.
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service: "docs", toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);
    expect(requests).toHaveLength(0);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    // 2) Enable the docs file resource for selected-doc: call succeeds.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      "docs",
      "selected-doc",
      "file",
      "Selected doc",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual({ documentId: "selected-doc" });
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-doc");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.arguments).toEqual({ document_id: "selected-doc" });
    expect(requests[0]!.nativeTool!.allowWrite).toBe(false);
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);

    // 3) Foreign document id: deny by file grant.
    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { document_id: "other-doc" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    // 4) Request pinned to the other tenant's connection: policy denial, unchanged requests.
    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { document_id: "selected-doc" },
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    // 5) Disable own file grant: deny again before the runner is reached.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      "docs",
      "selected-doc",
      "file",
      "Selected doc",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);
  },
);

const slidesReadToolCases = [
  {
    toolName: "slides_info",
    service: "slides",
    resourceArgument: "presentation_id",
    args: { presentation_id: "selected-deck" },
    result: { presentationId: "selected-deck" },
  },
  {
    toolName: "slides_read_slide",
    service: "slides",
    resourceArgument: "presentation_id",
    args: {
      presentation_id: "selected-deck",
      slide_id: "p:0",
      detail: true,
    },
    result: { presentationId: "selected-deck" },
  },
  {
    toolName: "slides_locate",
    service: "slides",
    resourceArgument: "presentation_id",
    args: {
      presentation_id: "selected-deck",
      text: "launch",
      all: true,
    },
    result: { presentationId: "selected-deck" },
  },
] as const;

it.each(slidesReadToolCases)(
  "Slides hosted reads: $toolName enforces file grants and tenant isolation",
  async ({ toolName, service, resourceArgument, args, result }) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    const tenant = await repo.bootstrapTenant("user_slides_files");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_slides_files",
      "user_slides_files@google.example.test",
      "Slides account",
      "[]",
    );
    const googleScope =
      toolName === "slides_info"
        ? "https://www.googleapis.com/auth/drive.file"
        : "https://www.googleapis.com/auth/presentations.readonly";
    const credential: StoredGoogleCredential = {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3_600_000).toISOString(),
      granted_scopes: ["openid", googleScope],
    };
    const encrypted = await cipher.encrypt(tenant.id, connection.id, credential);
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: connection.id,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });

    const otherTenant = await repo.bootstrapTenant("user_slides_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_slides_other",
      "user_slides_other@google.example.test",
      "Other slides account",
      "[]",
    );
    const otherEncrypted = await cipher.encrypt(otherTenant.id, otherConnection.id, credential);
    await repo.upsertCredential({
      tenantId: otherTenant.id,
      connectionId: otherConnection.id,
      ciphertext: otherEncrypted.ciphertext,
      nonce: otherEncrypted.nonce,
      keyVersion: Number(otherEncrypted.keyVersion),
    });
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      service,
      "other-deck",
      "file",
      "Other deck",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      {
        service,
        toolName,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);

    const token = await tokenFor("user_slides_files");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { ...args, ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual(result);
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-deck");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.arguments).toEqual(args);
    expect(requests[0]!.nativeTool!.allowWrite).toBe(false);
    expect(requests[0]!.nativeTool!.scopes).toContain(googleScope);
    if (toolName === "slides_info") {
      expect(requests[0]!.nativeTool!.scopes).not.toContain(
        "https://www.googleapis.com/auth/presentations.readonly",
      );
    }
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);
    expect(requests[0]!.service).toBe(service);

    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { ...args, [resourceArgument]: "other-deck" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: args,
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      true,
      "{}",
    );
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: false, allowDestructive: false, allowBillable: false },
    ]);
    const disabledToolBefore = requests.length;
    const disabledTool = await readResult(await call());
    expect(disabledTool.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledToolBefore);
  },
);

const workspaceWriteToolCases = [
  {
    toolName: "slides_replace_text",
    service: "slides",
    resourceArgument: "presentation_id",
    args: { presentation_id: "selected-deck", find: "Launch", replacement: "Ready", all: true },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "slides_replace_text",
    service: "slides",
    resourceArgument: "presentation_id",
    args: {
      presentation_id: "selected-deck",
      find: "Launch",
      replacement: "Ready",
      page_ids: ["p:0"],
    },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "slides_replace_text",
    service: "slides",
    resourceArgument: "presentation_id",
    args: {
      presentation_id: "selected-deck",
      find: "Launch",
      replacement: "Ready",
      object_id: "shape:one",
    },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "slides_replace_text",
    service: "slides",
    resourceArgument: "presentation_id",
    args: { presentation_id: "selected-deck", find: "Launch", replacement: "", all: true },
    result: { updated: true },
    allowDestructive: true,
  },
  {
    toolName: "slides_skip_slide",
    service: "slides",
    resourceArgument: "presentation_id",
    args: { presentation_id: "selected-deck", slide_id: "p:0" },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "slides_unskip_slide",
    service: "slides",
    resourceArgument: "presentation_id",
    args: { presentation_id: "selected-deck", slide_id: "p:0" },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "forms_add_question",
    service: "forms",
    resourceArgument: "form_id",
    args: { form_id: "selected-deck", title: "Question", type: "text", required: true },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "forms_move_question",
    service: "forms",
    resourceArgument: "form_id",
    args: { form_id: "selected-deck", old_index: 0, new_index: 1 },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "forms_delete_question",
    service: "forms",
    resourceArgument: "form_id",
    args: { form_id: "selected-deck", index: 0 },
    result: { updated: true },
    allowDestructive: true,
  },
  {
    toolName: "docs_add_tab",
    service: "docs",
    resourceArgument: "document_id",
    args: { document_id: "selected-deck", title: "New tab", index: 0 },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "docs_rename_tab",
    service: "docs",
    resourceArgument: "document_id",
    args: { document_id: "selected-deck", tab: "tab-1", title: "Renamed" },
    result: { updated: true },
    allowDestructive: false,
  },
  {
    toolName: "docs_delete_tab",
    service: "docs",
    resourceArgument: "document_id",
    args: { document_id: "selected-deck", tab: "tab-1" },
    result: { updated: true },
    allowDestructive: true,
  },
] as const;

it.each(workspaceWriteToolCases)(
  "Workspace hosted writes: $toolName enforces file grants, scopes, and tenant isolation",
  async ({ toolName, service, resourceArgument, args, result, allowDestructive }) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    const tenant = await repo.bootstrapTenant("user_slides_files");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_slides_files",
      "user_slides_files@google.example.test",
      "Slides account",
      "[]",
    );
    const googleScope = "https://www.googleapis.com/auth/drive.file";
    const credential: StoredGoogleCredential = {
      access_token: ACCESS_TOKEN,
      refresh_token: REFRESH_TOKEN,
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3_600_000).toISOString(),
      granted_scopes: ["openid", googleScope],
    };
    const encrypted = await cipher.encrypt(tenant.id, connection.id, credential);
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId: connection.id,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });

    const otherTenant = await repo.bootstrapTenant("user_slides_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_slides_other",
      "user_slides_other@google.example.test",
      "Other slides account",
      "[]",
    );
    const otherEncrypted = await cipher.encrypt(otherTenant.id, otherConnection.id, credential);
    await repo.upsertCredential({
      tenantId: otherTenant.id,
      connectionId: otherConnection.id,
      ciphertext: otherEncrypted.ciphertext,
      nonce: otherEncrypted.nonce,
      keyVersion: Number(otherEncrypted.keyVersion),
    });
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      service,
      "other-deck",
      "file",
      "Other deck",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      {
        service,
        toolName,
        enabled: true,
        allowDestructive: false,
        allowBillable: false,
      },
    ]);

    const token = await tokenFor("user_slides_files");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { ...args, ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: true, allowDestructive, allowBillable: false },
    ]);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual(result);
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-deck");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.arguments).toEqual(args);
    expect(requests[0]!.nativeTool!.allowWrite).toBe(true);
    expect(requests[0]!.nativeTool!.allowDestructive).toBe(allowDestructive);
    expect(requests[0]!.nativeTool!.scopes).toContain(googleScope);
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);
    expect(requests[0]!.service).toBe(service);

    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { ...args, [resourceArgument]: "other-deck" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: args,
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);

    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-deck",
      "file",
      "Selected deck",
      true,
      "{}",
    );
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: false, allowDestructive, allowBillable: false },
    ]);
    const disabledToolBefore = requests.length;
    const disabledTool = await readResult(await call());
    expect(disabledTool.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledToolBefore);
  },
);

it.each([
  {
    toolName: "drive_revisions_list",
    service: "drive",
    resourceArgument: "file_id",
    args: { file_id: "selected-resource" },
  },
  {
    toolName: "drive_revisions_get",
    service: "drive",
    resourceArgument: "file_id",
    args: { file_id: "selected-resource", revision_id: "revision-1" },
  },
  {
    toolName: "drive_comments_list",
    service: "drive",
    resourceArgument: "file_id",
    args: { file_id: "selected-resource" },
  },
  {
    toolName: "drive_comments_get",
    service: "drive",
    resourceArgument: "file_id",
    args: { file_id: "selected-resource", comment_id: "comment-1" },
  },
  {
    toolName: "forms_watch_list",
    service: "forms",
    resourceArgument: "form_id",
    args: { form_id: "selected-resource" },
  },
])(
  "comment/watch read $toolName enforces file grants and tenant isolation",
  async ({ toolName, service, resourceArgument, args }) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { resource: "selected-resource" } };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    // Owner tenant/connection (no file grant yet).
    const tenant = await repo.bootstrapTenant("user_selected_reads");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_docs_files",
      "user_docs_files@google.example.test",
      "Test account",
      "[]",
    );
    const encryptFor = async (tId: string, cId: string): Promise<void> => {
      const credential: StoredGoogleCredential = {
        access_token: ACCESS_TOKEN,
        refresh_token: REFRESH_TOKEN,
        token_type: "Bearer",
        expiry: new Date(Date.now() + 3_600_000).toISOString(),
        granted_scopes: ["openid", "https://www.googleapis.com/auth/drive.file"],
      };
      const encrypted = await cipher.encrypt(tId, cId, credential);
      await repo.upsertCredential({
        tenantId: tId,
        connectionId: cId,
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: Number(encrypted.keyVersion),
      });
    };
    await encryptFor(tenant.id, connection.id);

    // Foreign tenant with its own connection and a granted file ("foreign-resource").
    const otherTenant = await repo.bootstrapTenant("user_selected_reads_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_docs_other",
      "user_docs_other@google.example.test",
      "Other account",
      "[]",
    );
    await encryptFor(otherTenant.id, otherConnection.id);
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      service,
      "foreign-resource",
      "file",
      "Other doc",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);

    const token = await tokenFor("user_selected_reads");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { ...args, ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    // 1) Tool grant saved, but selected file has no grant yet: deny, zero dispatches.
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);
    expect(requests).toHaveLength(0);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    // 2) Enable the docs file resource for selected-doc: call succeeds.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual({ resource: "selected-resource" });
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-resource");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.arguments).toEqual(args);
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);

    // 3) Foreign document id: deny by file grant.
    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { ...args, [resourceArgument]: "foreign-resource" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    // 4) Request pinned to the other tenant's connection: policy denial, unchanged requests.
    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: args,
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    // 5) Disable own file grant: deny again before the runner is reached.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);
  },
);

it.each([
  {
    toolName: "sheets_chart_list",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource" },
  },
  {
    toolName: "sheets_named_ranges_list",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource" },
  },
  {
    toolName: "sheets_table_list",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource" },
  },
  {
    toolName: "sheets_banding_list",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource" },
  },
  {
    toolName: "sheets_conditional_format_list",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource" },
  },
])(
  "spreadsheet inventory $toolName enforces file grants and tenant isolation",
  async ({ toolName, service, resourceArgument, args }) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { resource: "selected-resource" } };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    // Owner tenant/connection (no file grant yet).
    const tenant = await repo.bootstrapTenant("user_sheet_inventory");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_docs_files",
      "user_docs_files@google.example.test",
      "Test account",
      "[]",
    );
    const encryptFor = async (tId: string, cId: string): Promise<void> => {
      const credential: StoredGoogleCredential = {
        access_token: ACCESS_TOKEN,
        refresh_token: REFRESH_TOKEN,
        token_type: "Bearer",
        expiry: new Date(Date.now() + 3_600_000).toISOString(),
        granted_scopes: ["openid", "https://www.googleapis.com/auth/drive.file"],
      };
      const encrypted = await cipher.encrypt(tId, cId, credential);
      await repo.upsertCredential({
        tenantId: tId,
        connectionId: cId,
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: Number(encrypted.keyVersion),
      });
    };
    await encryptFor(tenant.id, connection.id);

    // Foreign tenant with its own connection and a granted file ("foreign-resource").
    const otherTenant = await repo.bootstrapTenant("user_sheet_inventory_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_docs_other",
      "user_docs_other@google.example.test",
      "Other account",
      "[]",
    );
    await encryptFor(otherTenant.id, otherConnection.id);
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      service,
      "foreign-resource",
      "file",
      "Other doc",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);

    const token = await tokenFor("user_sheet_inventory");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { ...args, ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    // 1) Tool grant saved, but selected file has no grant yet: deny, zero dispatches.
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);
    expect(requests).toHaveLength(0);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    // 2) Enable the selected spreadsheet resource for selected-doc: call succeeds.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual({ resource: "selected-resource" });
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-resource");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.arguments).toEqual(args);
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);

    // 3) Foreign document id: deny by file grant.
    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { ...args, [resourceArgument]: "foreign-resource" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    // 4) Request pinned to the other tenant's connection: policy denial, unchanged requests.
    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: args,
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    // 5) Disable own file grant: deny again before the runner is reached.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);
  },
);

it.each([
  {
    toolName: "sheets_freeze",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource", sheet: "Sheet1", rows: 0 },
  },
  {
    toolName: "sheets_reorder_tab",
    service: "sheets",
    resourceArgument: "spreadsheet_id",
    args: { spreadsheet_id: "selected-resource", tab: "Sheet1", to: 0 },
  },
])(
  "spreadsheet layout $toolName enforces file grants and tenant isolation",
  async ({ toolName, service, resourceArgument, args }) => {
    const requests: ToolExecutionRequest[] = [];
    const env = await createEnv({
      __testToolRunner: {
        async execute(request) {
          requests.push(request);
          return { status: "ok", result: { resource: "selected-resource" } };
        },
      },
    });
    const repo = new HostedRepository(env.DB as unknown as Database);
    const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));

    // Owner tenant/connection (no file grant yet).
    const tenant = await repo.bootstrapTenant("user_sheet_layout");
    const connection = await repo.createConnection(
      tenant.id,
      "google-user_docs_files",
      "user_docs_files@google.example.test",
      "Test account",
      "[]",
    );
    const encryptFor = async (tId: string, cId: string): Promise<void> => {
      const credential: StoredGoogleCredential = {
        access_token: ACCESS_TOKEN,
        refresh_token: REFRESH_TOKEN,
        token_type: "Bearer",
        expiry: new Date(Date.now() + 3_600_000).toISOString(),
        granted_scopes: ["openid", "https://www.googleapis.com/auth/drive.file"],
      };
      const encrypted = await cipher.encrypt(tId, cId, credential);
      await repo.upsertCredential({
        tenantId: tId,
        connectionId: cId,
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: Number(encrypted.keyVersion),
      });
    };
    await encryptFor(tenant.id, connection.id);

    // Foreign tenant with its own connection and a granted file ("foreign-resource").
    const otherTenant = await repo.bootstrapTenant("user_sheet_layout_other");
    const otherConnection = await repo.createConnection(
      otherTenant.id,
      "google-user_docs_other",
      "user_docs_other@google.example.test",
      "Other account",
      "[]",
    );
    await encryptFor(otherTenant.id, otherConnection.id);
    await repo.upsertResourceGrant(
      otherTenant.id,
      otherConnection.id,
      service,
      "foreign-resource",
      "file",
      "Other doc",
      true,
      "{}",
    );
    await repo.saveToolGrants(otherTenant.id, otherConnection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);

    const token = await tokenFor("user_sheet_layout");
    const call = (overrides: Record<string, unknown> = {}) =>
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: { ...args, ...overrides },
        },
        { token },
      );
    const readResult = async (response: Response | Promise<Response>) => rpcJson(await response);

    // 1) Tool grant saved, but selected file has no grant yet: deny, zero dispatches.
    await repo.saveToolGrants(tenant.id, connection.id, [
      { service, toolName, enabled: true, allowDestructive: false, allowBillable: false },
    ]);
    expect(requests).toHaveLength(0);
    const beforeGrant = await readResult(await call());
    expect(beforeGrant.error.data.detail).toBe("grant_denied");
    expect(requests).toHaveLength(0);

    // 2) Enable the selected spreadsheet resource for selected-doc: call succeeds.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      true,
      "{}",
    );
    const ok = await readResult(await call());
    expect(ok.result.structuredContent).toEqual({ resource: "selected-resource" });
    expect(requests).toHaveLength(1);
    expect(requests[0]!.resourceType).toBe("file");
    expect(requests[0]!.resourceId).toBe("selected-resource");
    expect(requests[0]!.nativeTool!.name).toBe(toolName);
    expect(requests[0]!.nativeTool!.allowWrite).toBe(true);
    expect(requests[0]!.nativeTool!.allowDestructive).toBe(false);
    expect(requests[0]!.nativeTool!.arguments).toEqual(args);
    expect(requests[0]!.tenantId).toBe(tenant.id);
    expect(requests[0]!.connectionId).toBe(connection.id);

    // 3) Foreign document id: deny by file grant.
    const foreignBefore = requests.length;
    const foreign = await readResult(
      rpc(
        env,
        "tools/call",
        { name: toolName, arguments: { ...args, [resourceArgument]: "foreign-resource" } },
        { token },
      ),
    );
    expect(foreign.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(foreignBefore);

    // 4) Request pinned to the other tenant's connection: policy denial, unchanged requests.
    const crossBefore = requests.length;
    const cross = await readResult(
      rpc(
        env,
        "tools/call",
        {
          name: toolName,
          arguments: args,
          connectionId: otherConnection.id,
        },
        { token },
      ),
    );
    expect(cross.error.data.reason).toBe("policy_denied");
    expect(JSON.stringify(cross)).toMatch(/grant_denied|connection_not_found/);
    expect(requests.length).toBe(crossBefore);

    // 5) Disable own file grant: deny again before the runner is reached.
    await repo.upsertResourceGrant(
      tenant.id,
      connection.id,
      service,
      "selected-resource",
      "file",
      "Selected doc",
      false,
      "{}",
    );
    const disabledBefore = requests.length;
    const disabled = await readResult(await call());
    expect(disabled.error.data.detail).toBe("grant_denied");
    expect(requests.length).toBe(disabledBefore);
  },
);
