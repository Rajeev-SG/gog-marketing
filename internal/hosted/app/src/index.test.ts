/**
 * Worker route/session tests for the Clerk-protected hosted UI shell.
 *
 * Uses real Clerk SDK verification (`createClerkClient` + `authenticateRequest`)
 * with a locally generated RSA key pair (networkless via `jwtKey`) and
 * real Miniflare D1 for persistence — not mock strings.
 *
 * Internal tenant UUIDs are never asserted from HTML or API payloads; the
 * server-side Clerk→tenant mapping is inspected through the D1 repository.
 */
import { afterEach, describe, expect, it, beforeEach } from "vitest";
import { signJwt } from "@clerk/backend/jwt";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import type { Database } from "../../state/src/types.js";
import worker from "./index.js";
import type { Env } from "./index.js";
import {
  buildAuthenticatedRequest,
  buildCookieRequest,
  buildUnauthenticatedRequest,
  generateTestKey,
  signTestToken,
  type TestKeyPair,
} from "./test-helpers.js";

const WORKER_NAME = "hosted-app-native";
const ORIGIN = "https://test.example.com";
const CLERK_PUBLISHABLE_KEY = "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA";
const CLERK_ISSUER = "https://oriented-wombat-2813.clerk.accounts.dev";

let mf: Miniflare | undefined;
let keys: TestKeyPair;

beforeEach(async () => {
  keys = await generateTestKey();
});

describe("terminal Clerk failure browser callback outcomes (#62)", () => {
  const CODE = "cb-secret-code";
  const STATE = "cb-secret-state";
  const CALLBACK_URL = `${ORIGIN}/oauth/google/callback?code=${encodeURIComponent(CODE)}&state=${encodeURIComponent(STATE)}`;

  function browserCallback(url: string, headers?: Record<string, string>): Request {
    return new Request(url, {
      headers: { "Sec-Fetch-Dest": "document", Accept: "text/html", ...headers },
    });
  }

  function apiCallback(url: string, headers?: Record<string, string>): Request {
    return new Request(url, {
      headers: { Accept: "application/json", ...headers },
    });
  }

  async function expectNoDbSideEffects(env: Env): Promise<void> {
    const connections = await (env.DB as unknown as Database)
      .prepare("SELECT COUNT(*) AS n FROM hosted_google_connections")
      .first<{ n: number }>();
    expect(connections?.n).toBe(0);
    const tenants = await (env.DB as unknown as Database)
      .prepare("SELECT COUNT(*) AS n FROM hosted_tenants")
      .first<{ n: number }>();
    expect(tenants?.n).toBe(0);
  }

  /** Shared browser-notice contract: safe static HTML, cleaned query, no echo. */
  async function expectBrowserNotice(
    res: Response,
    expectedStatus: number,
    message: string,
  ): Promise<string> {
    expect(res.status).toBe(expectedStatus);
    expect(res.headers.get("content-type")).toContain("text/html");
    expect(res.headers.get("cache-control")).toBe("no-store");
    expect(res.headers.get("referrer-policy")).toBe("no-referrer");
    expect(res.headers.get("content-security-policy")).toBeTruthy();
    const html = await res.text();
    expect(html).toContain("gog-marketing");
    expect(html).toContain(message);
    expect(html).toContain('href="/"');
    // Client-side query cleanup instead of a redirect: no loop possible.
    expect(html).toContain("replaceState");
    // Never echo the secret-bearing OAuth parameters or raw auth errors.
    expect(html).not.toContain(CODE);
    expect(html).not.toContain(STATE);
    expect(html).not.toContain(CLERK_PUBLISHABLE_KEY);
    return html;
  }

  it("missing Clerk configuration: browser callback gets pure branded 503 HTML with no SDK load; API keeps safe JSON", async () => {
    const env = await createNativeEnv({
      CLERK_SECRET_KEY: "",
      CLERK_PUBLISHABLE_KEY: "",
    });
    const res = await worker.fetch(browserCallback(CALLBACK_URL), env);
    const html = await expectBrowserNotice(res, 503, "Sign-in is not available right now.");
    // Pure fallback shell: no Clerk SDK is loaded when configuration is invalid.
    expect(html).not.toContain("@clerk/clerk-js@6");
    expect(html).not.toContain("Authentication is not configured");
    await expectNoDbSideEffects(env);
    const api = await worker.fetch(apiCallback(CALLBACK_URL), env);
    expect(api.status).toBe(503);
    expect(api.headers.get("content-type")).toContain("application/json");
    const body = await api.text();
    expect(body).toBe('{"error":"Authentication is not configured."}');
    expect(body).not.toContain(CODE);
    expect(body).not.toContain(STATE);
  });

  it("provider authentication-service failure: browser callback gets branded 502 HTML; API keeps safe JSON", async () => {
    const env = await createNativeEnv();
    const headers = { Cookie: "__client_uat=1; __clerk_db_jwt=test_dev_browser" };
    // A corrupted handshake token makes the real Clerk SDK fail closed (502).
    const url = `${CALLBACK_URL}&__clerk_handshake=corrupted-handshake-token`;
    const res = await worker.fetch(browserCallback(url, headers), env);
    await expectBrowserNotice(res, 502, "We could not verify your sign-in session.");
    await expectNoDbSideEffects(env);
    const api = await worker.fetch(apiCallback(url, headers), env);
    expect(api.status).toBe(502);
    const body = await api.text();
    expect(body).toBe('{"error":"Authentication service is unavailable."}');
    expect(body).not.toContain(CODE);
    expect(body).not.toContain(STATE);
  });

  it("rejected session: browser callback gets branded 401 HTML; API keeps safe JSON", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      { sub: "user_rejected", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    // Tampered signature: the real SDK rejects the session fail-closed.
    const headers = {
      Cookie: `__session=${token}x; __client_uat=1; __clerk_db_jwt=test_dev_browser`,
    };
    const res = await worker.fetch(browserCallback(CALLBACK_URL, headers), env);
    const html = await expectBrowserNotice(res, 401, "Your session has ended.");
    expect(html).not.toContain(token);
    await expectNoDbSideEffects(env);
    const api = await worker.fetch(apiCallback(CALLBACK_URL, headers), env);
    expect(api.status).toBe(401);
    const body = await api.text();
    expect(body).toBe('{"error":"Authentication required."}');
    expect(body).not.toContain(token);
    expect(body).not.toContain(CODE);
    expect(body).not.toContain(STATE);
  });

  it("inactive tenant: browser callback gets branded 403 HTML; API keeps safe JSON", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      { sub: "user_inactive", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const headers = {
      Cookie: `__session=${token}; __client_uat=1; __clerk_db_jwt=test_dev_browser`,
    };
    // Bootstrap the tenant through the real auth path, then suspend it in
    // native D1 so the callback hits the inactive branch.
    const boot = await worker.fetch(new Request(`${ORIGIN}/api/tenant`, { headers }), env);
    expect(boot.status).toBe(200);
    await (env.DB as unknown as Database)
      .prepare("UPDATE hosted_tenants SET status = 'suspended' WHERE clerk_user_id = ?1")
      .bind("user_inactive")
      .run();
    const res = await worker.fetch(browserCallback(CALLBACK_URL, headers), env);
    await expectBrowserNotice(res, 403, "This workspace is not available.");
    const api = await worker.fetch(apiCallback(CALLBACK_URL, headers), env);
    expect(api.status).toBe(403);
    const body = await api.text();
    expect(body).toBe('{"error":"Tenant access is not available."}');
    expect(body).not.toContain(CODE);
    expect(body).not.toContain(STATE);
  });

  it("unexpected failure: browser callback gets branded 500 HTML; API keeps safe JSON", async () => {
    const env = await createNativeEnv();
    // Simulate an unexpected failure escaping route handling without mocking
    // the real SDK: the terminal notice branch is what is under test.
    const throwingEnv = new Proxy(env, {
      get(target, prop, receiver) {
        if (prop === "CLERK_SECRET_KEY") throw new Error("simulated unexpected failure");
        return Reflect.get(target, prop, receiver);
      },
    });
    const res = await worker.fetch(browserCallback(CALLBACK_URL), throwingEnv as Env);
    await expectBrowserNotice(res, 500, "Something went wrong.");
    const api = await worker.fetch(apiCallback(CALLBACK_URL), throwingEnv as Env);
    expect(api.status).toBe(500);
    const body = await api.text();
    expect(body).toBe('{"error":"Internal error."}');
    expect(body).not.toContain(CODE);
    expect(body).not.toContain(STATE);
  });
});

afterEach(async () => {
  await mf?.dispose();
  mf = undefined;
});

async function createNativeEnv(overrides?: Partial<Env>): Promise<Env> {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: WORKER_NAME,
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "hosted-app-native-test" },
          script: "export default {};",
          modules: true,
        },
      ],
    }),
  );
  const db = (await mf.getD1Database("DB")) as unknown as D1Database;

  // Apply migrations to the native D1.
  const migrationFiles = ["0001_initial_schema.sql", "0002_oauth_states_discovery.sql"];
  for (const file of migrationFiles) {
    const statements = splitMigrationStatements(loadMigrationSql(file));
    for (const statement of statements) {
      const result = await db.batch([db.prepare(statement)]);
      if (!result[0]?.success) throw new Error(`migration statement failed: ${statement}`);
    }
  }
  const baseEnv: Env = {
    DB: db,
    CLERK_SECRET_KEY: "sk_test_dummy_for_tests",
    CLERK_PUBLISHABLE_KEY: CLERK_PUBLISHABLE_KEY,
    CLERK_ISSUER: CLERK_ISSUER,
    CLERK_AUTHORIZED_PARTIES: ORIGIN,
    CLERK_JWT_KEY: keys.publicPem,
  };
  return { ...baseEnv, ...overrides };
}

async function authenticatedRequest(
  url: string,
  sub: string,
  env: Env,
  opts?: { method?: string; body?: string; azp?: string; iss?: string; exp?: number },
): Promise<Response> {
  const token = await signTestToken(
    {
      sub,
      azp: opts?.azp ?? ORIGIN,
      iss: opts?.iss ?? CLERK_ISSUER,
      exp: opts?.exp,
    },
    keys.privateJwk,
  );
  const req = buildAuthenticatedRequest(url, token, opts);
  return worker.fetch(req, env);
}

const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

async function tenantIdForClerkUser(env: Env, clerkUserId: string): Promise<string | null> {
  const row = await (env.DB as unknown as Database)
    .prepare("SELECT id FROM hosted_tenants WHERE clerk_user_id = ?1")
    .bind(clerkUserId)
    .first<{ id: string }>();
  return row?.id ?? null;
}

describe("hosted Clerk auth + tenant bootstrap (worker)", () => {
  it("redirects unauthenticated requests to the sign-in page with a CSP", async () => {
    const env = await createNativeEnv();
    const req = buildUnauthenticatedRequest(`${ORIGIN}/`);
    const res = await worker.fetch(req, env);
    expect(res.status).toBe(200);
    const html = await res.text();
    expect(html).toContain("sign-in");
    expect(html).toContain("Clerk");
    expect(html).toContain("@clerk/clerk-js@6");
    expect(res.headers.get("content-security-policy")).toContain("script-src");
  });

  it("serves least-privilege Clerk bot/fraud CSP directives on sign-in and cookie-authenticated home", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      { sub: "user_csp", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    for (const request of [
      buildUnauthenticatedRequest(`${ORIGIN}/`),
      buildCookieRequest(`${ORIGIN}/`, token, true),
    ]) {
      const res = await worker.fetch(request, env);
      expect(res.status).toBe(200);
      const policy = res.headers.get("content-security-policy");
      expect(policy).toBeTruthy();
      const directives = Object.fromEntries(
        policy!.split(";").map((directive) => {
          const [name, ...sources] = directive.trim().split(/\s+/);
          return [name, sources];
        }),
      );
      // Exact source sets catch missing required hosts, port wildcards in the
      // wrong directive, and accidental broad scheme/all-origin allowances.
      expect(directives["script-src"]).toEqual([
        "'self'",
        "'unsafe-inline'",
        CLERK_ISSUER,
        "https://challenges.cloudflare.com",
        "https://*.protect.clerk.com",
      ]);
      expect(directives["connect-src"]).toEqual([
        "'self'",
        CLERK_ISSUER,
        "wss://oriented-wombat-2813.clerk.accounts.dev",
        "https://*.protect.clerk.com:*",
      ]);
      expect(directives["img-src"]).toEqual([
        "'self'",
        "data:",
        CLERK_ISSUER,
        "https://img.clerk.com",
      ]);
      expect(directives["worker-src"]).toEqual(["'self'", "blob:"]);
      expect(directives["frame-src"]).toEqual([
        CLERK_ISSUER,
        "https://challenges.cloudflare.com",
        "https://*.protect.clerk.com",
      ]);
      expect(directives["default-src"]).toEqual(["'self'"]);
      expect(directives["object-src"]).toEqual(["'none'"]);
      expect(directives["frame-ancestors"]).toEqual(["'none'"]);
      expect(directives["form-action"]).toEqual(["'self'", CLERK_ISSUER]);
    }
  });

  it("returns 401 for unauthenticated API requests", async () => {
    const env = await createNativeEnv();
    const req = buildUnauthenticatedRequest(`${ORIGIN}/api/tenant`);
    const res = await worker.fetch(req, env);
    expect(res.status).toBe(401);
    const body = (await res.json()) as { error?: string };
    expect(body.error).toContain("Authentication required");
    expect(body.error).not.toContain("sk_");
    expect(body.error).not.toContain("token");
  });

  it("never exposes the internal tenant UUID in HTML or API payloads", async () => {
    const env = await createNativeEnv();
    const res = await authenticatedRequest(`${ORIGIN}/`, "user_real_test", env);
    expect(res.status).toBe(200);
    const html = await res.text();
    // Server-side mapping: the verified Clerk user gets a stable tenant.
    const tenantId = await tenantIdForClerkUser(env, "user_real_test");
    expect(tenantId).toBeTruthy();
    expect(tenantId).toMatch(UUID_RE);
    // ...but the UUID never leaks into the rendered HTML.
    expect(html).not.toMatch(UUID_RE);
    const repo = new HostedRepository(env.DB as unknown as Database);
    const tenant = await repo.getTenantByClerkId("user_real_test");
    expect(tenant?.clerkUserId).toBe("user_real_test");

    const apiRes = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_real_test", env);
    expect(apiRes.status).toBe(200);
    const body = (await apiRes.json()) as { status?: string; userId?: string };
    expect(body.status).toBe("active");
    expect(body.userId).toBe("user_real_test");
    expect(body).not.toHaveProperty("tenantId");
    expect(JSON.stringify(body)).not.toContain(tenantId ?? "");
  });

  it("renders real Clerk UI + sign-out on the signed-in home", async () => {
    const env = await createNativeEnv();
    const res = await authenticatedRequest(`${ORIGIN}/`, "user_logout", env);
    expect(res.status).toBe(200);
    const html = await res.text();
    expect(html).toContain("@clerk/ui@1");
    expect(html).toContain("@clerk/clerk-js@6");
    expect(html).toContain("Clerk.load");
    expect(html).toContain("Clerk.mountUserButton(");
    expect(html).toContain("Clerk.signOut(");
    expect(html).toContain('id="sign-out"');
  });

  it("returns the same tenant on repeated sign-in (idempotent)", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_repeat", env);
    const id1 = await tenantIdForClerkUser(env, "user_repeat");
    expect(id1).toBeTruthy();

    await authenticatedRequest(`${ORIGIN}/`, "user_repeat", env);
    const id2 = await tenantIdForClerkUser(env, "user_repeat");
    expect(id2).toBe(id1);
  });

  it("parallel first-login callers for the same user resolve to one stable tenant (native D1)", async () => {
    const env = await createNativeEnv();
    const callers = 4;
    const results = await Promise.all(
      Array.from({ length: callers }, () =>
        authenticatedRequest(`${ORIGIN}/`, "user_race_native", env),
      ),
    );
    for (const res of results) {
      expect(res.status).toBe(200);
      const html = await res.text();
      // No tenant UUID is rendered on the home page.
      expect(html).not.toMatch(UUID_RE);
      expect(html).toContain("ready");
    }
    const row = await (env.DB as unknown as Database)
      .prepare("SELECT COUNT(*) AS n FROM hosted_tenants WHERE clerk_user_id = ?1")
      .bind("user_race_native")
      .first<{ n: number }>();
    expect(row?.n).toBe(1);
  });

  it("rejects a tampered foreign tenant hint in the body", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_tenant_a", env);
    const tenantIdA = await tenantIdForClerkUser(env, "user_tenant_a");
    expect(tenantIdA).toBeTruthy();

    const resB = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_tenant_b", env, {
      method: "POST",
      body: JSON.stringify({ tenantId: tenantIdA }),
    });
    expect(resB.status).toBe(200);
    const body = (await resB.json()) as { userId?: string; status?: string };
    expect(body.userId).toBe("user_tenant_b");
    expect(JSON.stringify(body)).not.toContain(tenantIdA ?? "");
    const tenantIdB = await tenantIdForClerkUser(env, "user_tenant_b");
    expect(tenantIdB).toBeTruthy();
    expect(tenantIdB).not.toBe(tenantIdA);
  });

  it("rejects a tampered foreign tenant hint in query parameters", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_q_a", env);
    const tenantIdA = await tenantIdForClerkUser(env, "user_q_a");
    expect(tenantIdA).toBeTruthy();

    const resB = await authenticatedRequest(
      `${ORIGIN}/api/tenant?tenantId=${tenantIdA}`,
      "user_q_b",
      env,
    );
    expect(resB.status).toBe(200);
    const body = (await resB.json()) as { userId?: string };
    expect(body.userId).toBe("user_q_b");
    expect(JSON.stringify(body)).not.toContain(tenantIdA ?? "");
  });

  it("rejects a tampered foreign tenant hint in headers", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_h_a", env);
    const tenantIdA = await tenantIdForClerkUser(env, "user_h_a");
    expect(tenantIdA).toBeTruthy();

    const token = await signTestToken(
      { sub: "user_h_b", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const req = new Request(`${ORIGIN}/api/tenant`, {
      headers: new Headers({
        Authorization: `Bearer ${token}`,
        "X-Tenant-Id": tenantIdA ?? "",
        Origin: ORIGIN,
      }),
    });
    const resB = await worker.fetch(req, env);
    expect(resB.status).toBe(200);
    const body = (await resB.json()) as { userId?: string };
    expect(body.userId).toBe("user_h_b");
    expect(JSON.stringify(body)).not.toContain(tenantIdA ?? "");
  });

  it("rejects an expired Clerk session token", async () => {
    const env = await createNativeEnv();
    const past = Math.floor(Date.now() / 1000) - 100;
    const res = await authenticatedRequest(`${ORIGIN}/`, "user_expired", env, {
      exp: past,
    });
    const body = await res.text();
    expect(body).not.toContain("ready");
    expect(body).not.toContain("Signed in");
    expect(body).not.toContain("user_expired");
    expect(body).not.toContain("sk_test");
  });

  it("rejects a wrong authorized party (azp) claim", async () => {
    const env = await createNativeEnv();
    const res = await authenticatedRequest(`${ORIGIN}/`, "user_azp_wrong", env, {
      azp: "https://evil.example.com",
    });
    const body = await res.text();
    expect(body).not.toContain("ready");
    expect(body).not.toContain("Signed in");
    expect(body).not.toContain("user_azp_wrong");
    expect(body).not.toContain("evil");
  });

  it("rejects a session token issued by a different issuer (SDK leaves iss unenforced)", async () => {
    const env = await createNativeEnv();
    // Same key, same audience/azp, but `iss` claims a different tenant.
    const res = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_iss_wrong", env, {
      iss: "https://evil-wombat-2813.clerk.accounts.dev",
    });
    expect(res.status).toBe(401);
    const body = await res.text();
    expect(body).not.toContain("user_iss_wrong");
    expect(body).not.toContain("evil-wombat");

    const homeRes = await authenticatedRequest(`${ORIGIN}/`, "user_iss_wrong", env, {
      iss: "https://evil-wombat-2813.clerk.accounts.dev",
    });
    const html = await homeRes.text();
    expect(html).not.toContain("ready");
    expect(html).not.toContain("user_iss_wrong");
  });

  it("fails closed when CLERK_ISSUER is missing even with a valid session", async () => {
    const env = await createNativeEnv({ CLERK_ISSUER: undefined });
    const res = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_no_issuer", env);
    expect(res.status).toBe(503);
    const body = (await res.json()) as { error?: string };
    expect(body.error).toContain("not configured");
  });

  it("fails closed on a malformed CLERK_ISSUER configuration", async () => {
    const env = await createNativeEnv({ CLERK_ISSUER: "not-a-url" });
    const res = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_bad_issuer", env);
    expect(res.status).toBe(503);

    // A syntactically valid URL on a different Clerk domain is still a
    // misconfiguration: it must never trust tokens from another issuer.
    const env2 = await createNativeEnv({
      CLERK_ISSUER: "https://other-instance.clerk.accounts.dev",
    });
    const res2 = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_bad_issuer", env2);
    expect(res2.status).toBe(503);
  });

  it("fails closed when CLERK_AUTHORIZED_PARTIES is empty or malformed", async () => {
    const env = await createNativeEnv({ CLERK_AUTHORIZED_PARTIES: "" });
    const res = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_no_parties", env);
    expect(res.status).toBe(503);

    const env2 = await createNativeEnv({
      CLERK_AUTHORIZED_PARTIES: "https://test.example.com,not-a-url",
    });
    const res2 = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_bad_parties", env2);
    expect(res2.status).toBe(503);
  });

  it("rejects a tampered JWT (invalid signature)", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      { sub: "user_tamper", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const parts = token.split(".");
    const tampered = `${parts[0]}.${btoa('{"sub":"user_attacker","azp":"https://test.example.com","exp":9999999999,"iat":1000000000,"nbf":1000000000,"sid":"sess_t","iss":"https://oriented-wombat-2813.clerk.accounts.dev","aud":"test_audience"}')}.tampered_sig`;
    const req = buildAuthenticatedRequest(`${ORIGIN}/`, tampered);
    const res = await worker.fetch(req, env);
    const body = await res.text();
    expect(body).not.toContain("ready");
    expect(body).not.toContain("Signed in");
    expect(body).not.toContain("user_attacker");
    expect(body).not.toContain("user_tamper");
  });

  it("error responses never echo secret keys or raw tokens", async () => {
    const env = await createNativeEnv();
    const tamperedToken = "not.a.valid.token";
    const req = buildAuthenticatedRequest(`${ORIGIN}/api/tenant`, tamperedToken);
    const res = await worker.fetch(req, env);
    const body = await res.text();
    expect(body).not.toContain("sk_test");
    expect(body).not.toContain(tamperedToken);
    expect(body).not.toContain("Bearer");
    expect(body).not.toContain("__session");
  });

  it("tenant persists across worker restarts (native D1)", async () => {
    const env1 = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_persist", env1);
    const repo = new HostedRepository(env1.DB as unknown as Database);
    const tenant = await repo.getTenantByClerkId("user_persist");
    expect(tenant).not.toBeNull();
    expect(tenant?.id).toMatch(UUID_RE);
  });

  it("different Clerk users get different tenants (via the repository)", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_diff_a", env);
    await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_diff_b", env);
    const idA = await tenantIdForClerkUser(env, "user_diff_a");
    const idB = await tenantIdForClerkUser(env, "user_diff_b");
    expect(idA).toBeTruthy();
    expect(idB).toBeTruthy();
    expect(idA).not.toBe(idB);
  });

  it("denies home and API access for suspended and deleted tenants even with a valid JWT", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_suspended", env);
    await (env.DB as unknown as Database)
      .prepare("UPDATE hosted_tenants SET status = 'suspended' WHERE clerk_user_id = ?1")
      .bind("user_suspended")
      .run();

    const homeRes = await authenticatedRequest(`${ORIGIN}/`, "user_suspended", env);
    expect(homeRes.status).toBe(403);
    const homeHtml = await homeRes.text();
    expect(homeHtml).not.toContain("ready");
    expect(homeHtml).not.toContain("user_suspended");

    const apiRes = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_suspended", env);
    expect(apiRes.status).toBe(403);
    const apiBody = (await apiRes.json()) as { error?: string };
    expect(apiBody.error).not.toContain("suspended");
    expect(apiBody.error).not.toContain("user_suspended");
  });

  it("denies access for deleted tenants and recovers when the tenant is reactivated", async () => {
    const env = await createNativeEnv();
    await authenticatedRequest(`${ORIGIN}/`, "user_deleted", env);
    await (env.DB as unknown as Database)
      .prepare("UPDATE hosted_tenants SET status = 'deleted' WHERE clerk_user_id = ?1")
      .bind("user_deleted")
      .run();
    const apiRes = await authenticatedRequest(`${ORIGIN}/api/tenant`, "user_deleted", env);
    expect(apiRes.status).toBe(403);

    // Operator-driven recovery: once the tenant is active again the session
    // works without re-authentication.
    await (env.DB as unknown as Database)
      .prepare("UPDATE hosted_tenants SET status = 'active' WHERE clerk_user_id = ?1")
      .bind("user_deleted")
      .run();
    const homeRes = await authenticatedRequest(`${ORIGIN}/`, "user_deleted", env);
    expect(homeRes.status).toBe(200);
    const html = await homeRes.text();
    expect(html).toContain("ready");
    expect(html).not.toMatch(UUID_RE);
  });

  it("authenticates real session cookies and keeps repeated Clerk-to-tenant mapping private", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      { sub: "user_cookie", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const request = buildCookieRequest(`${ORIGIN}/`, token, true);
    expect(request.headers.has("authorization")).toBe(false);
    const home = await worker.fetch(request, env);
    expect(home.status).toBe(200);
    const html = await home.text();
    expect(html).toContain("Your workspace is ready.");
    expect(html).not.toContain(token);
    expect(html).not.toContain("test_dev_browser");
    const first = await tenantIdForClerkUser(env, "user_cookie");
    expect(first).toMatch(UUID_RE);
    expect(html).not.toContain(first!);
    const api = await worker.fetch(buildCookieRequest(`${ORIGIN}/api/tenant`, token), env);
    expect(api.status).toBe(200);
    expect(await api.json()).toEqual({ status: "active", userId: "user_cookie" });
    expect(await tenantIdForClerkUser(env, "user_cookie")).toBe(first);
  });

  it("expired session cookies fail safely without creating a tenant or echoing cookies", async () => {
    const env = await createNativeEnv();
    const token = await signTestToken(
      {
        sub: "user_cookie_expired",
        azp: ORIGIN,
        iss: CLERK_ISSUER,
        exp: Math.floor(Date.now() / 1000) - 100,
      },
      keys.privateJwk,
    );
    // Fetch/API requests are not eligible for Clerk's document handshake.
    const api = await worker.fetch(buildCookieRequest(`${ORIGIN}/api/tenant`, token), env);
    expect(api.status).toBe(401);
    const body = await api.text();
    expect(body).toBe('{"error":"Authentication required."}');
    expect(body).not.toContain(token);
    expect(body).not.toContain("test_dev_browser");
    expect(await tenantIdForClerkUser(env, "user_cookie_expired")).toBeNull();
    const navigation = await worker.fetch(buildCookieRequest(`${ORIGIN}/`, token, true), env);
    expect(navigation.status).toBe(307);
    expect(navigation.headers.get("cache-control")).toBe("no-store");
    const handshakeUrl = new URL(navigation.headers.get("location")!);
    expect(handshakeUrl.origin).toBe(CLERK_ISSUER);
    expect(handshakeUrl.pathname).toBe("/v1/client/handshake");
    expect(await navigation.text()).toBe("");
    // Once the browser removes its session cookies, it gets sign-in, not home.
    const signedOut = await worker.fetch(buildUnauthenticatedRequest(`${ORIGIN}/`), env);
    expect(await signedOut.text()).toContain('id="sign-in"');
  });

  it("forwards real SDK handshake redirect headers and completes a signed handshake into cookie auth", async () => {
    const env = await createNativeEnv();
    const request = new Request(`${ORIGIN}/`, {
      headers: { "Sec-Fetch-Dest": "document", Accept: "text/html" },
    });
    const redirect = await worker.fetch(request, env);
    expect(redirect.status).toBe(307);
    expect(redirect.headers.get("cache-control")).toBe("no-store");
    const location = new URL(redirect.headers.get("location")!);
    expect(location.origin).toBe(CLERK_ISSUER);
    expect(location.pathname).toBe("/v1/client/handshake");
    expect(location.searchParams.get("redirect_url")).toBe(`${ORIGIN}/`);
    expect(location.searchParams.get("__clerk_hs_reason")).toBe("dev-browser-missing");
    expect(await redirect.text()).toBe("");
    expect(await tenantIdForClerkUser(env, "user_handshake")).toBeNull();

    // Emulate the Clerk FAPI return with a real signed handshake JWT. Both
    // the handshake and its nested session are verified by the installed SDK.
    const session = await signTestToken(
      { sub: "user_handshake", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const directive = `__session=${session}; Path=/; Secure; SameSite=Lax`;
    const handshake = await signJwt({ handshake: [directive] }, keys.privateJwk, {
      algorithm: "RS256",
    });
    const callback = new Request(`${ORIGIN}/?__clerk_handshake=${encodeURIComponent(handshake!)}`, {
      headers: { "Sec-Fetch-Dest": "document", Accept: "text/html" },
    });
    const completed = await worker.fetch(callback, env);
    expect(completed.status).toBe(307);
    expect(completed.headers.get("set-cookie")).toContain(directive);
    expect(completed.headers.get("location")).toBe(`${ORIGIN}/`);
    expect(completed.headers.get("cache-control")).toBe("no-store");
    expect(await completed.text()).not.toContain(session);
    const cookie = completed.headers.get("set-cookie")!.split(";")[0];
    const next = await worker.fetch(
      new Request(`${ORIGIN}/api/tenant`, {
        headers: { Cookie: `${cookie}; __client_uat=1; __clerk_db_jwt=test_dev_browser` },
      }),
      env,
    );
    expect(next.status).toBe(200);
    expect(await next.json()).toEqual({ status: "active", userId: "user_handshake" });
  });

  it("unknown routes return 404", async () => {
    const env = await createNativeEnv();
    const req = buildUnauthenticatedRequest(`${ORIGIN}/nonexistent`);
    const res = await worker.fetch(req, env);
    expect(res.status).toBe(404);
  });
});

describe("hosted Google connection routes (#62)", () => {
  const CREDENTIAL_KEY = Buffer.alloc(32, 5).toString("base64");

  function googleFetchStub(options?: {
    token?: Record<string, unknown> | Error;
    userinfo?: Record<string, unknown> | Error;
  }) {
    const tokenCalls: string[] = [];
    const userinfoCalls: string[] = [];
    const impl = (async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "https://oauth2.googleapis.com/token") {
        if (options?.token instanceof Error) throw options.token;
        tokenCalls.push(url);
        return new Response(
          JSON.stringify(
            options?.token ?? {
              access_token: "ya29.route-access",
              refresh_token: "1//route-refresh-secret",
              token_type: "Bearer",
              expires_in: 3600,
              scope: "openid email https://www.googleapis.com/auth/analytics.readonly",
            },
          ),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      if (url === "https://openidconnect.googleapis.com/v1/userinfo") {
        if (options?.userinfo instanceof Error) throw options.userinfo;
        userinfoCalls.push(url);
        return new Response(
          JSON.stringify(
            options?.userinfo ?? {
              sub: "google-sub-1",
              email: "owner@example.com",
              email_verified: true,
              name: "Owner",
            },
          ),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      throw new Error(`unexpected fetch: ${url}`);
    }) as typeof fetch;
    return { fetch: impl, tokenCalls, userinfoCalls };
  }

  async function createGoogleEnv(
    overrides?: Partial<Env>,
    stub?: ReturnType<typeof googleFetchStub>,
  ): Promise<Env> {
    return createNativeEnv({
      GOG_GOOGLE_OAUTH_CLIENT_ID:
        "629716276051-cq3jnl899hj4ie3f3vhokke8aebc4ff8.apps.googleusercontent.com",
      GOG_GOOGLE_OAUTH_CLIENT_SECRET: "test-secret",
      GOG_GOOGLE_OAUTH_REDIRECT_URI:
        "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback",
      GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: CREDENTIAL_KEY,
      __testFetch: stub?.fetch,
      ...overrides,
    });
  }

  async function googleRequest(
    url: string,
    sub: string,
    opts?: { method?: string; body?: string },
  ): Promise<Request> {
    const token = await signTestToken({ sub, azp: ORIGIN, iss: CLERK_ISSUER }, keys.privateJwk);
    const request = buildAuthenticatedRequest(url, token, opts);
    request.headers.set("Content-Type", "application/json");
    return request;
  }

  it("requires Clerk authentication for Google routes", async () => {
    const env = await createGoogleEnv();
    const list = await worker.fetch(
      buildUnauthenticatedRequest(`${ORIGIN}/api/google/connections`),
      env,
    );
    expect(list.status).toBe(401);
    expect(await list.json()).toEqual({ error: "Authentication required." });
    const callback = await worker.fetch(
      buildUnauthenticatedRequest(`${ORIGIN}/oauth/google/callback?code=c&state=s`),
      env,
    );
    expect(callback.status).toBe(401);
    const invalidBearerRequest = buildAuthenticatedRequest(
      `${ORIGIN}/api/google/connect`,
      "unused",
      { method: "POST", body: "{}" },
    );
    invalidBearerRequest.headers.set("Content-Type", "application/json");
    const connect = await worker.fetch(invalidBearerRequest, env);
    expect(connect.status).toBe(401);
  });

  it("fails closed with a distinct operator-configuration outcome", async () => {
    const env = await createGoogleEnv({
      GOG_GOOGLE_OAUTH_CLIENT_ID: undefined,
      GOG_GOOGLE_OAUTH_CLIENT_SECRET: undefined,
      GOG_GOOGLE_OAUTH_REDIRECT_URI: undefined,
      GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: undefined,
    });
    const res = await worker.fetch(
      await googleRequest(`${ORIGIN}/api/google/connect`, "user_config", {
        method: "POST",
        body: "{}",
      }),
      env,
    );
    expect(res.status).toBe(503);
    const body = (await res.json()) as { code: string; error: string };
    expect(body.code).toBe("operator_config_missing");
    expect(body.error).toBe("Google connection is not configured by the operator yet.");
  });

  it("runs the full route flow: start, verified callback, safe list, secret-free output", async () => {
    const stub = googleFetchStub();
    const env = await createGoogleEnv({}, stub);

    const start = await worker.fetch(
      await googleRequest(`${ORIGIN}/api/google/connect`, "user_google", {
        method: "POST",
        body: JSON.stringify({ services: ["analytics"] }),
      }),
      env,
    );
    expect(start.status).toBe(200);
    const startBody = (await start.json()) as { authorizationUrl: string; connectionId: string };
    expect(startBody.authorizationUrl).toContain("accounts.google.com");
    expect(startBody.authorizationUrl).toContain("code_challenge_method=S256");
    expect(startBody.authorizationUrl).toContain("prompt=consent");

    const callbackUrl = new URL(startBody.authorizationUrl);
    const callback = await worker.fetch(
      await googleRequest(
        `${ORIGIN}/oauth/google/callback?code=route-code&state=${encodeURIComponent(callbackUrl.searchParams.get("state")!)}`,
        "user_google",
      ),
      env,
    );
    expect(callback.status).toBe(200);
    const callbackBody = (await callback.json()) as {
      connection: { id: string; email: string; status: string };
      discovery: { status: string; detail: string };
    };
    expect(callbackBody.connection.status).toBe("active");
    expect(callbackBody.connection.email).toBe("owner@example.com");
    expect(callbackBody.discovery.status).toBe("unavailable");
    expect(callbackBody.discovery.detail).toBe("runner_not_configured");
    expect(stub.userinfoCalls.length).toBe(1);

    // Credential is ciphertext-only at rest; plaintext never reaches D1.
    const stored = await (env.DB as unknown as Database)
      .prepare(
        "SELECT ciphertext, nonce, key_version FROM hosted_connection_credentials WHERE connection_id = ?1",
      )
      .bind(callbackBody.connection.id)
      .first<{ ciphertext: Uint8Array; nonce: Uint8Array; key_version: number }>();
    expect(stored).not.toBeNull();
    const storedText = Buffer.from(stored!.ciphertext).toString("utf8");
    expect(storedText).not.toContain("1//route-refresh-secret");
    expect(stored!.key_version).toBe(1);

    // Listing is tenant-scoped and secret-free.
    const list = await worker.fetch(
      await googleRequest(`${ORIGIN}/api/google/connections`, "user_google"),
      env,
    );
    expect(list.status).toBe(200);
    const listBody = (await list.json()) as { connections: Array<Record<string, unknown>> };
    expect(listBody.connections).toHaveLength(1);
    const serialized = JSON.stringify(listBody);
    expect(serialized).not.toContain("1//route-refresh-secret");
    expect(serialized).not.toContain("ya29.route-access");
    // The internal tenant UUID is never exposed; the per-connection id is
    // the stable client-side handle used by disconnect/refresh actions.
    expect(listBody.connections[0]!.id).toBe(callbackBody.connection.id);
    const tenantId = await tenantIdForClerkUser(env, "user_google");
    expect(serialized).not.toContain(tenantId!);
    expect(listBody.connections[0]!.id).toBe(callbackBody.connection.id);

    // Replay of the same state is rejected distinctly.
    const replay = await worker.fetch(
      await googleRequest(
        `${ORIGIN}/oauth/google/callback?code=route-code&state=${encodeURIComponent(callbackUrl.searchParams.get("state")!)}`,
        "user_google",
      ),
      env,
    );
    expect(replay.status).toBe(409);
    expect(((await replay.json()) as { code: string }).code).toBe("state_replayed");

    // Home surfaces the Google accounts panel.
    const home = await worker.fetch(await googleRequest(`${ORIGIN}/`, "user_google"), env);
    expect(await home.text()).toContain("Connect Google account");
  });

  it("surfaces Google consent denial as a distinct outcome", async () => {
    const env = await createGoogleEnv({});
    const start = await worker.fetch(
      await googleRequest(`${ORIGIN}/api/google/connect`, "user_denied", {
        method: "POST",
        body: "{}",
      }),
      env,
    );
    expect(start.status).toBe(200);
    const startBody = (await start.json()) as { authorizationUrl: string };
    const state = new URL(startBody.authorizationUrl).searchParams.get("state")!;
    const denied = await worker.fetch(
      await googleRequest(
        `${ORIGIN}/oauth/google/callback?error=access_denied&state=${encodeURIComponent(state)}`,
        "user_denied",
      ),
      env,
    );
    expect(denied.status).toBe(403);
    const body = (await denied.json()) as { code: string; error: string };
    expect(body.code).toBe("consent_denied");
    expect(body.error).toBe("Google consent was denied, so the account was not connected.");
    // The pending stub was discarded.
    const tenantId = await tenantIdForClerkUser(env, "user_denied");
    const rows = await (env.DB as unknown as Database)
      .prepare("SELECT COUNT(*) AS n FROM hosted_google_connections WHERE tenant_id = ?1")
      .bind(tenantId!)
      .first<{ n: number }>();
    expect(rows?.n).toBe(0);
  });
  it("rejects cookie CSRF and invalid connect JSON before database or provider side effects", async () => {
    const stub = googleFetchStub();
    const env = await createGoogleEnv({}, stub);
    const token = await signTestToken(
      { sub: "user_csrf", azp: ORIGIN, iss: CLERK_ISSUER },
      keys.privateJwk,
    );
    const repo = new HostedRepository(env.DB);
    const tenant = await repo.bootstrapTenant("user_csrf");
    const connection = await repo.createConnection(
      tenant.id,
      "sub-existing",
      "existing@example.com",
      "Existing",
      "[]",
    );
    const before = await repo.listConnections(tenant.id);
    const cookie = buildCookieRequest(ORIGIN, token).headers.get("cookie")!;
    for (const path of [
      "/api/google/connect",
      `/api/google/connections/${connection.id}/refresh`,
      `/api/google/connections/${connection.id}/disconnect`,
    ]) {
      for (const origin of [undefined, "https://evil.test.example.com"]) {
        const response = await worker.fetch(
          new Request(ORIGIN + path, {
            method: "POST",
            headers: {
              Cookie: cookie,
              "Content-Type": "application/json",
              ...(origin ? { Origin: origin } : {}),
            },
            body: "{}",
          }),
          env,
        );
        expect(response.status).toBe(403);
        expect(await repo.listConnections(tenant.id)).toEqual(before);
        expect(stub.tokenCalls).toHaveLength(0);
        expect(stub.userinfoCalls).toHaveLength(0);
      }
    }
    for (const [body, contentType] of [
      ["not-json", "application/json"],
      ["[]", "application/json"],
      ["services=analytics", "application/x-www-form-urlencoded"],
    ]) {
      const response = await worker.fetch(
        new Request(ORIGIN + "/api/google/connect", {
          method: "POST",
          headers: { Cookie: cookie, Origin: ORIGIN, "Content-Type": contentType },
          body,
        }),
        env,
      );
      expect(response.status).toBe(400);
      expect(await repo.listConnections(tenant.id)).toEqual(before);
    }
    const legitimate = await worker.fetch(
      new Request(ORIGIN + "/api/google/connect", {
        method: "POST",
        headers: { Cookie: cookie, Origin: ORIGIN, "Content-Type": "application/json" },
        body: "{}",
      }),
      env,
    );
    expect(legitimate.status).toBe(200);
    const bearerHeaders = (
      await googleRequest(ORIGIN + "/api/google/connect", "user_csrf", {
        method: "POST",
        body: "{}",
      })
    ).headers;
    bearerHeaders.delete("origin");
    const bearer = await worker.fetch(
      new Request(ORIGIN + "/api/google/connect", {
        method: "POST",
        headers: bearerHeaders,
        body: "{}",
      }),
      env,
    );
    expect(bearer.status).toBe(200);
  });

  it("redirects browser callback success/errors home with only a fixed safe code; API remains JSON", async () => {
    const env = await createGoogleEnv({}, googleFetchStub());
    const start = await worker.fetch(
      await googleRequest(ORIGIN + "/api/google/connect", "user_browser", {
        method: "POST",
        body: "{}",
      }),
      env,
    );
    const data = (await start.json()) as { authorizationUrl: string };
    const state = new URL(data.authorizationUrl).searchParams.get("state")!;
    const callbackUrl =
      ORIGIN + "/oauth/google/callback?code=private-code&state=" + encodeURIComponent(state);
    const apiRequest = await googleRequest(callbackUrl, "user_browser");
    const headers = new Headers(apiRequest.headers);
    headers.set("Accept", "text/html");
    const browser = await worker.fetch(new Request(callbackUrl, { headers }), env);
    expect(browser.status).toBe(303);
    expect(browser.headers.get("location")).toBe("/?google=connected");
    expect(await browser.text()).toBe("");
    const replay = await worker.fetch(new Request(callbackUrl, { headers }), env);
    expect(replay.headers.get("location")).toBe("/?google=state_replayed");
    expect(await replay.text()).toBe("");
    const api = await worker.fetch(apiRequest, env);
    expect(api.status).toBe(409);
    expect(((await api.json()) as { code: string }).code).toBe("state_replayed");
    const home = await worker.fetch(
      await googleRequest(ORIGIN + "/?google=state_replayed", "user_browser"),
      env,
    );
    const html = await home.text();
    expect(html).toContain("Reconnect");
    expect(html).toContain("connectionId: connection.id");
    expect(html).toContain("Google account connected.");
    expect(html).not.toContain(state);
    expect(html).not.toContain("private-code");
  });

  it("uses only canonical runner bindings for an authenticated typed discovery request", async () => {
    const { buildConnectDeps } = await import("./connect.js");
    const captured: Array<{ url: string; init?: RequestInit }> = [];
    const keyPair = (await crypto.subtle.generateKey(
      {
        name: "RSASSA-PKCS1-v1_5",
        modulusLength: 2048,
        publicExponent: new Uint8Array([1, 0, 1]),
        hash: "SHA-256",
      },
      true,
      ["sign", "verify"],
    )) as CryptoKeyPair;
    const pkcs8 = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
    const exp = Math.floor((Date.now() + 300_000) / 1000);
    const nativeIdPayload = Buffer.from(JSON.stringify({ exp })).toString("base64url");
    const env = await createGoogleEnv({
      GOG_CLOUD_RUN_SERVICE_URL: "https://runner.example.com/v1/execute",
      GOG_RUNNER_WIF_PROVIDER:
        "projects/629716276051/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
      GOG_RUNNER_WIF_ISSUER: "https://gog-marketing.rajeev-sgill.workers.dev",
      GOG_RUNNER_WIF_KEY_ID: "public-test-kid",
      GOG_RUNNER_WIF_SIGNING_KEY: Buffer.from(pkcs8).toString("base64"),
      GOG_RUNNER_SERVICE_ACCOUNT: "gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com",
      GOG_RUNNER_INVOCATION_TOKEN: "invocation-secret-at-least-32-bytes",
      __testFetch: (async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        captured.push({ url, init });
        if (url === "https://sts.googleapis.com/v1/token") {
          return Response.json({ access_token: "federated-access" });
        }
        if (url.includes(":generateIdToken")) {
          return Response.json({ token: `native-header.${nativeIdPayload}.native-signature` });
        }
        return Response.json({
          request_id: "req-test-runner",
          operation: "discover",
          ok: true,
          result: {
            resources: [],
            statuses: {
              analytics: { state: "ok", resource_count: 0, checked_at: "2026-10-06T00:00:00Z" },
            },
          },
          duration_ms: 12,
        });
      }) as typeof fetch,
    });
    const outcome = buildConnectDeps(env, new HostedRepository(env.DB));
    expect(outcome.ok).toBe(true);
    if (!outcome.ok) return;
    const result = await outcome.deps.discovery.discover({
      tenantId: "trusted-tenant",
      connectionId: "trusted-connection",
      services: ["analytics"],
      accessToken: "not-browser-output",
      googleEmail: "verified@example.com",
      googleSubject: "google-subject",
    });
    expect(result.status).toBe("empty");
    expect(result.detail).toBe("no_resources");
    expect(captured).toHaveLength(3);
    expect(captured[2]!.url).toBe("https://runner.example.com/v1/execute");
    const headers = new Headers(captured[2]!.init?.headers);
    expect(headers.get("content-type")).toBe("application/json");
    expect(headers.get("authorization")).toMatch(
      /^Bearer [A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/,
    );
    expect(headers.get("x-serverless-authorization")).toMatch(
      /^Bearer native-header\.[A-Za-z0-9_-]+\.native-signature$/,
    );
    expect(JSON.parse(new TextDecoder().decode(captured[2]!.init?.body as Uint8Array))).toEqual({
      tenant_id: "trusted-tenant",
      connection_id: "trusted-connection",
      operation: "discover",
      google_email: "verified@example.com",
      google_subject: "google-subject",
      access_token: "not-browser-output",
      services: ["analytics"],
    });
  });
});
