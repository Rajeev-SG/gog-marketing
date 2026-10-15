/** Actual Workerd + native Agents protocol; only Clerk's remote BAPI is a fixture. */
import { afterAll, beforeAll, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { generateTestKey, signTestToken } from "../test-helpers.js";

let mf: Miniflare;
let bundleDir: string;
let cookie: string;
let sessionStatus = "active";
let sessionChecks = 0;
const outboundRoutes: string[] = [];
const owner = "user_native_chat_owner";
const origin = "https://test.example.com";

beforeAll(async () => {
  const keys = await generateTestKey();
  const token = await signTestToken(
    { sub: owner, azp: origin, iss: "https://oriented-wombat-2813.clerk.accounts.dev" },
    keys.privateJwk,
  );
  cookie = "__session=" + token + "; __client_uat=1; __clerk_db_jwt=test_dev_browser";
  // Use the production bundler's Node compatibility shims, not a separate test-only bundler.
  bundleDir = mkdtempSync(join(tmpdir(), "gog-native-chat-"));
  mkdirSync(join(bundleDir, "assets"));
  execFileSync(
    process.execPath,
    [
      fileURLToPath(new URL("../../node_modules/wrangler/bin/wrangler.js", import.meta.url).href),
      "deploy",
      "--dry-run",
      "--config",
      fileURLToPath(new URL("../../wrangler.toml", import.meta.url).href),
      "--outdir",
      bundleDir,
      "--assets",
      join(bundleDir, "assets"),
    ],
    { timeout: 15000, stdio: "pipe", env: { ...process.env, NODE_ENV: "production" } },
  );
  const script = readFileSync(join(bundleDir, "worker.js"), "utf8");
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "native-integrated-chat",
          modules: true,
          script,
          compatibilityDate: "2026-09-18",
          compatibilityFlags: ["nodejs_compat"],
          d1Databases: { DB: "native-integrated-chat" },
          durableObjects: { Chat: { className: "Chat", useSQLite: true } },
          bindings: {
            CLERK_SECRET_KEY: "sk_test_native_fixture_only",
            CLERK_PUBLISHABLE_KEY: "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA",
            CLERK_ISSUER: "https://oriented-wombat-2813.clerk.accounts.dev",
            CLERK_AUTHORIZED_PARTIES: origin,
            CLERK_JWT_KEY: keys.publicPem,
            CHAT_OWNER_USER_ID: owner,
          },
          outboundService: async (request: Request) => {
            const url = new URL(request.url);
            outboundRoutes.push(url.origin + url.pathname);
            if (url.origin !== "https://api.clerk.com" || !url.pathname.startsWith("/v1/sessions/"))
              throw new Error("Unexpected native fixture network request");
            sessionChecks++;
            return Response.json({
              object: "session",
              id: "sess_test_123",
              client_id: "client_native",
              user_id: owner,
              status: sessionStatus,
              expire_at: Date.now() + 3600000,
              abandon_at: Date.now() + 3600000,
              last_active_at: Date.now(),
              created_at: Date.now(),
              updated_at: Date.now(),
              actor: null,
            });
          },
        },
      ],
    }),
  );
  const db = await mf.getD1Database("DB");
  for (const sql of splitMigrationStatements(loadMigrationSql("0001_initial_schema.sql")))
    await db.prepare(sql).run();
}, 20000);
afterAll(async () => {
  await mf?.dispose();
  if (bundleDir) rmSync(bundleDir, { recursive: true, force: true });
});

it("uses the real owner namespace and blocks an already-open socket after Clerk revocation", async () => {
  const meta = await mf.dispatchFetch(origin + "/api/chat/session", {
    headers: { Cookie: cookie },
  });
  const payload = (await meta.json()) as { conversation: string };
  expect(meta.status, JSON.stringify(payload) + JSON.stringify(outboundRoutes)).toBe(200);
  const { conversation } = payload;
  const path = origin + "/agents/chat/" + conversation;
  const history = await mf.dispatchFetch(path + "/get-messages", { headers: { Cookie: cookie } });
  expect(history.status).toBe(200);
  expect(history.headers.get("cache-control")).toBe("no-store");
  expect(await history.json()).toEqual([]);
  const upgrade = await mf.dispatchFetch(path, {
    headers: { Cookie: cookie, Origin: origin, Upgrade: "websocket" },
  });
  expect(upgrade.status).toBe(101);
  const socket = upgrade.webSocket!;
  socket.accept();
  const reply = new Promise<Record<string, unknown>>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("Native RPC did not reply")), 3000);
    socket.addEventListener("message", (event) => {
      const data = JSON.parse(String(event.data));
      if (data.id === "native-proof") {
        clearTimeout(timer);
        resolve(data);
      }
    });
  });
  socket.send(
    JSON.stringify({ type: "rpc", id: "native-proof", method: "markStopped", args: [[]] }),
  );
  expect(await reply).toMatchObject({ type: "rpc", success: true });
  const checksBefore = sessionChecks;
  sessionStatus = "revoked";
  const closed = new Promise<number>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("Revoked socket remained open")), 3000);
    socket.addEventListener("close", (event) => {
      clearTimeout(timer);
      resolve(event.code);
    });
  });
  socket.send(
    JSON.stringify({ type: "rpc", id: "revoked-proof", method: "markStopped", args: [[]] }),
  );
  expect(await closed).toBe(4401);
  expect(sessionChecks).toBeGreaterThan(checksBefore);
  const deniedHistory = await mf.dispatchFetch(path + "/get-messages", {
    headers: { Cookie: cookie },
  });
  expect(deniedHistory.status).toBe(401);
}, 10000);
