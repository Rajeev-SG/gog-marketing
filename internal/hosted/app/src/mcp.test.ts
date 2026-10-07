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
import worker from "./index.js";
import type { Env } from "./index.js";
import { generateTestKey, signTestToken, type TestKeyPair } from "./test-helpers.js";

const RESOURCE = "https://mcp.example.test/mcp";
const ORIGIN = "https://mcp.example.test";
const CLERK_PUBLISHABLE_KEY = "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA";
const CLERK_ISSUER = "https://oriented-wombat-2813.clerk.accounts.dev";

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
  for (const file of ["0001_initial_schema.sql", "0002_oauth_states_discovery.sql"]) {
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
  options: { token?: string | null; id?: string | number | null; body?: string } = {},
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
    expect(await missing.json()).toMatchObject({
      jsonrpc: "2.0",
      error: { code: -32001, data: { reason: "invalid_request" } },
    });

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

  it("policy-filters tools/call and never executes it in #65", async () => {
    const env = await createEnv();
    await grantService(env, "user_policy", "analytics", true);
    const token = await tokenFor("user_policy");

    const denied = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "tagmanager_accounts_list", arguments: { tenantId: "other" } },
        { token },
      ),
    );
    expect(denied).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      error: { code: -32602, data: { reason: "policy_denied" } },
    });

    const notImplemented = await rpcJson(
      await rpc(
        env,
        "tools/call",
        { name: "analytics_report", arguments: { property: "properties/foreign" } },
        { token },
      ),
    );
    expect(notImplemented).toMatchObject({
      jsonrpc: "2.0",
      id: "req-1",
      error: {
        code: -32001,
        data: { reason: "not_yet_implemented", issue: 66 },
      },
    });
  });

  it("returns JSON-RPC parse, invalid-request, and method-not-found errors", async () => {
    const env = await createEnv();
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
