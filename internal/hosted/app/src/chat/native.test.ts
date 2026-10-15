/** Actual Workerd/native Agents protocol; remote Clerk and OpenRouter are explicit fixtures. */
import { afterAll, beforeAll, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { generateTestKey, signTestToken, type TestKeyPair } from "../test-helpers.js";

let mf: Miniflare;
let bundleDir: string;
let cookie: string;
let keys: TestKeyPair;
const revoked = new Set<string>();
const expiries = new Map<string, number>();
let providerCalls = 0;
let rateLimited = false;
let sessionChecks = 0;
const outboundRoutes: string[] = [];
const owner = "user_native_chat_owner";
const origin = "https://test.example.com";

beforeAll(async () => {
  keys = await generateTestKey();
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
            OPENROUTER_API_KEY: "fixture-only-not-a-real-provider-key",
          },
          outboundService: async (request: Request) => {
            const url = new URL(request.url);
            outboundRoutes.push(url.origin + url.pathname);
            if (
              url.origin === "https://openrouter.ai" &&
              url.pathname === "/api/v1/chat/completions"
            ) {
              providerCalls++;
              const chunk = {
                id: "fixture-reply",
                object: "chat.completion.chunk",
                created: 1,
                model: "openai/gpt-4.1-mini",
              };
              const chunks: Record<string, unknown>[] = Array.from(
                "Native fixture reply.",
                (letter, index) => ({
                  ...chunk,
                  choices: [
                    {
                      index: 0,
                      delta: { ...(index === 0 ? { role: "assistant" } : {}), content: letter },
                      finish_reason: null,
                    },
                  ],
                }),
              );
              chunks.push({
                ...chunk,
                choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
                usage: { prompt_tokens: 12, completion_tokens: 4, total_tokens: 16 },
              });
              const body = new ReadableStream<Uint8Array>({
                async start(controller) {
                  for (const value of chunks) {
                    controller.enqueue(
                      new TextEncoder().encode("data: " + JSON.stringify(value) + "\n\n"),
                    );
                    await new Promise((resolve) => setTimeout(resolve, 20));
                  }
                  controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
                  controller.close();
                },
              });
              return new Response(body, { headers: { "Content-Type": "text/event-stream" } });
            }
            if (url.origin !== "https://api.clerk.com" || !url.pathname.startsWith("/v1/sessions/"))
              throw new Error("Unexpected native fixture network request");
            const sessionId = url.pathname.split("/").at(-1)!;
            sessionChecks++;
            if (rateLimited)
              return Response.json(
                { errors: [{ code: "rate_limit_exceeded", message: "Fixture BAPI limit" }] },
                { status: 429 },
              );
            return Response.json({
              object: "session",
              id: sessionId,
              client_id: "client_native",
              user_id: owner,
              status: revoked.has(sessionId) ? "revoked" : "active",
              expire_at: expiries.get(sessionId) ?? Date.now() + 3600000,
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

type Socket = NonNullable<Awaited<ReturnType<Miniflare["dispatchFetch"]>>["webSocket"]>;
type Frame = {
  id?: string;
  type?: string;
  done?: boolean;
  error?: boolean;
  success?: boolean;
  messages?: { id: string; metadata?: { gogStopped?: boolean } }[];
};

function frame(socket: Socket, predicate: (value: Frame) => boolean): Promise<Frame> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      socket.removeEventListener("message", listener);
      reject(new Error("Expected native frame did not arrive"));
    }, 5000);
    const listener = (event: { data: unknown }) => {
      const value = JSON.parse(String(event.data)) as Frame;
      if (predicate(value)) {
        clearTimeout(timer);
        socket.removeEventListener("message", listener);
        resolve(value);
      }
    };
    socket.addEventListener("message", listener);
  });
}
function closed(socket: Socket): Promise<number> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("Unauthorized socket remained open")), 5000);
    socket.addEventListener("close", (event) => {
      clearTimeout(timer);
      resolve(event.code);
    });
  });
}
async function cookieFor(sessionId: string) {
  const jwt = await signTestToken(
    {
      sub: owner,
      sid: sessionId,
      azp: origin,
      iss: "https://oriented-wombat-2813.clerk.accounts.dev",
    },
    keys.privateJwk,
  );
  return "__session=" + jwt + "; __client_uat=1; __clerk_db_jwt=test_dev_browser";
}
async function open(path: string, sessionCookie: string) {
  const response = await mf.dispatchFetch(path + "?_pk=" + crypto.randomUUID(), {
    headers: { Cookie: sessionCookie, Origin: origin, Upgrade: "websocket" },
  });
  expect(response.status).toBe(101);
  const socket = response.webSocket!;
  socket.accept();
  const ready = frame(socket, (value) => value.type === "cf_agent_stream_resume_none");
  socket.send(
    JSON.stringify({ type: "cf_agent_stream_resume_request", probeId: crypto.randomUUID() }),
  );
  await ready;
  return socket;
}
async function history(path: string, sessionCookie: string) {
  const response = await mf.dispatchFetch(path + "/get-messages", {
    headers: { Cookie: sessionCookie },
  });
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  return (await response.json()) as {
    id: string;
    role: string;
    parts: { type: string; text?: string }[];
    metadata?: { gogStopped?: boolean };
  }[];
}
async function turn(socket: Socket, id: string, messages: unknown[]) {
  const done = frame(
    socket,
    (value) =>
      value.type === "cf_agent_use_chat_response" &&
      value.id === id &&
      !!(value.done || value.error),
  );
  socket.send(
    JSON.stringify({
      type: "cf_agent_use_chat_request",
      id,
      init: { method: "POST", body: JSON.stringify({ messages, trigger: "submit-message" }) },
    }),
  );
  expect(await done).not.toHaveProperty("error", true);
}

it("keeps native streams/history intact while excluding silent revoked and expired recipients", async () => {
  const meta = await mf.dispatchFetch(origin + "/api/chat/session", {
    headers: { Cookie: cookie },
  });
  const payload = (await meta.json()) as { conversation: string };
  expect(meta.status, JSON.stringify(payload) + JSON.stringify(outboundRoutes)).toBe(200);
  const path = origin + "/agents/chat/" + payload.conversation;
  expect(await history(path, cookie)).toEqual([]);
  const socketA = await open(path, cookie);
  const cookieB = await cookieFor("sess_native_B");
  const socketB = await open(path, cookieB);
  expect(socketA.readyState).toBe(1);
  const seenA: string[] = [];
  socketA.addEventListener("message", (event) => seenA.push(String(event.data)));
  // A remains completely silent after revocation; B must not leak any new content to it.
  const checksBefore = sessionChecks;
  revoked.add("sess_test_123");
  const closedA = closed(socketA);
  await turn(socketB, "turn-one", [
    { id: "u1", role: "user", parts: [{ type: "text", text: "B-only native fixture message" }] },
  ]);
  expect(await closedA).toBe(4401);
  expect(seenA.join("")).not.toContain("B-only native fixture message");
  expect(sessionChecks).toBeGreaterThan(checksBefore);
  expect(providerCalls).toBe(1);
  // Paced token chunks must not cause a BAPI lookup per token/frame.
  expect(sessionChecks - checksBefore).toBeLessThan(15);
  const first = await history(path, cookieB);
  expect(first).toHaveLength(2);
  expect(first[1].parts.some((part) => part.text === "Native fixture reply.")).toBe(true);

  const cookieC = await cookieFor("sess_native_C");
  expiries.set("sess_native_C", Date.now() + 200);
  const socketC = await open(path, cookieC);
  const seenC: string[] = [];
  socketC.addEventListener("message", (event) => seenC.push(String(event.data)));
  await new Promise((resolve) => setTimeout(resolve, 250));
  const closedC = closed(socketC);
  await turn(socketB, "turn-two", [
    ...first,
    { id: "u2", role: "user", parts: [{ type: "text", text: "After silent session expiry" }] },
  ]);
  expect(await closedC).toBe(4401);
  expect(seenC.join("")).not.toContain("After silent session expiry");
  const beforeStop = await history(path, cookieB);
  expect(beforeStop).toHaveLength(4);
  const fullSnapshot = frame(
    socketB,
    (value) =>
      value.type === "cf_agent_chat_messages" &&
      !!value.messages?.some((message) => message.id === "u2" && message.metadata?.gogStopped),
  );
  const rpc = frame(socketB, (value) => value.id === "stop-proof");
  socketB.send(
    JSON.stringify({ type: "rpc", id: "stop-proof", method: "markStopped", args: [["u2"]] }),
  );
  expect(await rpc).toMatchObject({ success: true });
  expect((await fullSnapshot).messages!.map((message) => message.id)).toEqual(
    beforeStop.map((message) => message.id),
  );
  expect(
    (await history(path, cookieB)).find((message) => message.id === "u2")?.metadata?.gogStopped,
  ).toBe(true);
  const denied = await mf.dispatchFetch(path + "/get-messages", { headers: { Cookie: cookie } });
  expect(denied.status).toBe(401);

  // Five individually valid CJK messages are >100 KB UTF-8 but <100,000 UTF-16 units.
  const unicode = Array.from({ length: 5 }, (_, n) => ({
    id: "unicode-" + n,
    role: "user",
    parts: [{ type: "text", text: "汉".repeat(8000) }],
  }));
  const packet = JSON.stringify({
    type: "cf_agent_use_chat_request",
    id: "oversized",
    init: {
      method: "POST",
      body: JSON.stringify({ messages: unicode, trigger: "submit-message" }),
    },
  });
  expect(packet.length).toBeLessThan(100000);
  expect(new TextEncoder().encode(packet).byteLength).toBeGreaterThan(100000);
  const callsBefore = providerCalls;
  const closedB = closed(socketB);
  socketB.send(packet);
  expect(await closedB).toBe(4400);
  expect(providerCalls).toBe(callsBefore);
  const cookieD = await cookieFor("sess_native_D");
  const socketD = await open(path, cookieD);
  rateLimited = true;
  const retryableClose = closed(socketD);
  socketD.send(
    JSON.stringify({ type: "rpc", id: "rate-limit-proof", method: "markStopped", args: [[]] }),
  );
  expect(await retryableClose).toBe(1013);
  rateLimited = false;
  expect(
    (await mf.dispatchFetch(origin + "/api/chat/session", { headers: { Cookie: cookieD } })).status,
  ).toBe(200);

  expect((await history(path, cookieB)).map((message) => message.id)).toEqual(
    beforeStop.map((message) => message.id),
  );
}, 15000);
