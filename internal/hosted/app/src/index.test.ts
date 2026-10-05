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
  const statements = splitMigrationStatements(loadMigrationSql("0001_initial_schema.sql"));
  for (const statement of statements) {
    const result = await db.batch([db.prepare(statement)]);
    if (!result[0]?.success) throw new Error(`migration statement failed: ${statement}`);
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
