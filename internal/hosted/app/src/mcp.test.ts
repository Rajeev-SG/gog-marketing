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
    "resource",
    service,
    enabled,
    "{}",
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
  const cipher = new CredentialCipher(parseCredentialKeys(CREDENTIAL_KEY));
  const credential: StoredGoogleCredential = {
    access_token: ACCESS_TOKEN,
    refresh_token: REFRESH_TOKEN,
    token_type: "Bearer",
    expiry: new Date(Date.now() + 3_600_000).toISOString(),
    granted_scopes: [],
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
    expect(namesA).toContain("analytics_report");
    expect(namesA).not.toContain("googleads_query");
    expect(namesA).not.toContain("tagmanager_accounts_list");
    expect(JSON.stringify(listA)).not.toContain("user_a");

    const listB = await rpcJson(await rpc(env, "tools/list", undefined, { token: tokenB }));
    const namesB = listB.result.tools.map((tool: { name: string }) => tool.name);
    expect(namesB).toContain("tagmanager_accounts_list");
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
        detail: "unsupported_tool_mapping",
        params: {
          name: "analytics_report",
          arguments: { property: fixture.resourceId },
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
