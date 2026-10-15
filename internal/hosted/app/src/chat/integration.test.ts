/** Integration checks: real Clerk-format JWT verification and native D1, fake BAPI only. */
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { HostedRepository } from "../../../state/src/repository.js";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import type { Database } from "../../../state/src/types.js";
import worker, { type Env } from "../index.js";
import {
  buildCookieRequest,
  generateTestKey,
  signTestToken,
  type TestKeyPair,
} from "../test-helpers.js";

const origin = "https://test.example.com";
const issuer = "https://oriented-wombat-2813.clerk.accounts.dev";
const owner = "user_chat_owner";
let mf: Miniflare;
let env: Env;
let keys: TestKeyPair;
let repo: HostedRepository;
let expiresAt: number;
const assets = vi.fn((_request: Request) =>
  Promise.resolve(
    new Response('<html><head></head><body><div id="root"></div></body></html>', {
      headers: { "content-type": "text/html" },
    }),
  ),
);

beforeAll(async () => {
  keys = await generateTestKey();
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "chat-integration",
          modules: true,
          compatibilityDate: "2024-12-01",
          script: "export default { fetch() { return new Response('unused'); } };",
          d1Databases: { DB: "chat-integration" },
        },
      ],
    }),
  );
  const db = await mf.getD1Database("DB");
  for (const statement of splitMigrationStatements(loadMigrationSql("0001_initial_schema.sql"))) {
    await db.prepare(statement).run();
  }
  env = {
    DB: db as unknown as D1Database,
    CLERK_SECRET_KEY: "sk_test_chat_fixture_only",
    CLERK_PUBLISHABLE_KEY: "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA",
    CLERK_ISSUER: issuer,
    CLERK_AUTHORIZED_PARTIES: origin,
    CLERK_JWT_KEY: keys.publicPem,
    CHAT_OWNER_USER_ID: owner,
    ASSETS: { fetch: assets } as unknown as Fetcher,
  };
  repo = new HostedRepository(db as unknown as Database);
});
afterEach(() => {
  vi.unstubAllGlobals();
  assets.mockClear();
});
afterAll(async () => {
  await mf?.dispose();
});

async function signed(path: string, user = owner, extraHeaders: Record<string, string> = {}) {
  const token = await signTestToken({ sub: user, azp: origin, iss: issuer }, keys.privateJwk);
  const cookie = buildCookieRequest(origin + path, token);
  const headers = new Headers(cookie.headers);
  for (const [name, value] of Object.entries(extraHeaders)) headers.set(name, value);
  return new Request(cookie, { headers });
}
function clerkSession(status = "active", userId = owner, expiry = Date.now() + 3600000) {
  expiresAt = expiry;
  const network = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(
      typeof input === "string" ? input : input instanceof URL ? input : input.url,
    );
    if (url.origin !== "https://api.clerk.com" || !url.pathname.startsWith("/v1/sessions/")) {
      throw new Error("Unexpected fixture network request");
    }
    return Response.json({
      object: "session",
      id: "sess_test_123",
      client_id: "client_test",
      user_id: userId,
      status,
      expire_at: expiry,
      abandon_at: expiry,
      created_at: Date.now() - 60000,
      updated_at: Date.now(),
      last_active_at: Date.now(),
      actor: null,
    });
  });
  vi.stubGlobal("fetch", network);
  return network;
}

describe("integrated chat identity boundary", () => {
  it("rejects a suspended tenant even with a valid owner session", async () => {
    const tenant = await repo.bootstrapTenant(owner);
    const network = clerkSession();
    await env.DB.prepare("UPDATE hosted_tenants SET status = 'suspended' WHERE id = ?")
      .bind(tenant.id)
      .run();
    try {
      const response = await worker.fetch(await signed("/api/chat/session"), env);
      expect(response.status).toBe(503);
      expect(network).not.toHaveBeenCalled();
    } finally {
      await env.DB.prepare("UPDATE hosted_tenants SET status = 'active' WHERE id = ?")
        .bind(tenant.id)
        .run();
    }
  });

  it("fails closed without owner configuration but leaves the existing tenant API working", async () => {
    const request = await signed("/api/chat/session");
    expect((await worker.fetch(request, { ...env, CHAT_OWNER_USER_ID: "" })).status).toBe(503);
    const existing = await worker.fetch(await signed("/api/tenant"), {
      ...env,
      CHAT_OWNER_USER_ID: "",
    });
    expect(existing.status).toBe(200);
    expect(await existing.json()).toMatchObject({ userId: owner, status: "active" });
  });

  it.each(["/api/chat/session", "/agents/chat/owner/get-messages", "/agents/chat/owner"])(
    "rejects anonymous %s without model/history access",
    async (path) => {
      const network = clerkSession();
      const response = await worker.fetch(new Request(origin + path), env);
      expect(response.status).toBe(401);
      expect(network).not.toHaveBeenCalled();
      expect(assets).not.toHaveBeenCalled();
    },
  );

  it("rejects a valid different Clerk user before checking the owner's session", async () => {
    const network = clerkSession();
    expect(
      (await worker.fetch(await signed("/api/chat/session", "user_another"), env)).status,
    ).toBe(403);
    expect(network).not.toHaveBeenCalled();
  });

  it("returns only safe owner session metadata with an opaque conversation and millisecond expiry", async () => {
    clerkSession();
    const response = await worker.fetch(await signed("/api/chat/session"), env);
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toBe("no-store");
    const data = (await response.json()) as Record<string, unknown>;
    expect(data).toMatchObject({ model: "openai/gpt-4.1-mini", expiresAt });
    expect(data.conversation).toMatch(/^[a-f0-9]{64}$/);
    const tenant = await repo.getTenantByClerkId(owner);
    const serialized = JSON.stringify(data);
    expect(serialized).not.toContain(tenant!.id);
    expect(serialized).not.toContain("sk_test");
    expect(serialized).not.toContain("sess_test");
    expect(serialized).not.toContain(owner);
  });

  it.each(["revoked", "expired", "ended"])(
    "rejects a still-signed JWT when the Clerk session is %s",
    async (status) => {
      clerkSession(status);
      expect((await worker.fetch(await signed("/api/chat/session"), env)).status).toBe(401);
    },
  );

  it("rejects an expired active-session timestamp rather than multiplying its milliseconds", async () => {
    clerkSession("active", owner, Date.now() - 1000);
    expect((await worker.fetch(await signed("/api/chat/session"), env)).status).toBe(401);
  });

  it("rejects a live session belonging to a different user", async () => {
    clerkSession("active", "user_foreign");
    expect((await worker.fetch(await signed("/api/chat/session"), env)).status).toBe(401);
  });

  it("does not expose raw provider errors if Clerk refuses the session check", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          { errors: [{ message: "private-fixture-do-not-echo", code: "invalid" }] },
          { status: 401 },
        ),
      ),
    );
    const response = await worker.fetch(await signed("/api/chat/session"), env);
    expect(response.status).toBe(502);
    expect(await response.text()).not.toContain("private-fixture-do-not-echo");
  });

  it("denies forged conversation history before touching the native agent namespace", async () => {
    clerkSession();
    const response = await worker.fetch(
      await signed("/agents/chat/" + "a".repeat(64) + "/get-messages"),
      env,
    );
    expect(response.status).toBe(404);
    expect(assets).not.toHaveBeenCalled();
  });

  it("denies cross-origin sockets and unknown agent classes", async () => {
    clerkSession();
    const metadata = (await (
      await worker.fetch(await signed("/api/chat/session"), env)
    ).json()) as { conversation: string };
    const response = await worker.fetch(
      await signed("/agents/chat/" + metadata.conversation, owner, {
        Origin: "https://evil.example",
        Upgrade: "websocket",
      }),
      env,
    );
    expect(response.status).toBe(403);
    const unknown = await worker.fetch(await signed("/agents/other/owner"), env);
    expect(unknown.status).toBe(404);
  });

  it("serves the owner chat HTML with Clerk and never serves raw chat.html publicly", async () => {
    clerkSession();
    const response = await worker.fetch(await signed("/chat"), env);
    expect(response.status).toBe(200);
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(response.headers.get("content-security-policy")).toContain("wss://test.example.com");
    expect(await response.text()).toContain("clerk.browser.js");
    expect(assets).toHaveBeenCalledTimes(1);
    expect(new URL(assets.mock.calls[0]![0].url).pathname).toBe("/chat.html");
    expect((await worker.fetch(new Request(origin + "/chat.html"), env)).status).toBe(404);
  });
});
