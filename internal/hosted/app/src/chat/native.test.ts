/** Actual Workerd/native Agents protocol; remote Clerk and OpenRouter are explicit fixtures. */
import { afterAll, beforeAll, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { HostedRepository } from "../../../state/src/repository.js";
import { generateTestKey, signTestToken, type TestKeyPair } from "../test-helpers.js";

let mf: Miniflare;
let bundleDir: string;
let cookie: string;
let keys: TestKeyPair;
const revoked = new Set<string>();
const expiries = new Map<string, number>();
let providerCalls = 0;
let nextToolCall: { id: string; connectionId: string } | null = null;
let rateLimited = false;
let sessionChecks = 0;
let delayedSessionCheck: string | undefined;
const outboundRoutes: string[] = [];
const owner = "user_native_chat_owner";
const cohortA = "user_NativeCohortA";
const cohortB = "user_NativeCohortB";
const sessionUsers = new Map<string, string>();
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
            CHAT_COHORT_USER_IDS: cohortA + "," + cohortB,
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
              const payload = (await request.clone().json()) as {
                provider: unknown;
                model: string;
                models?: unknown;
              };
              expect(payload.provider).toMatchObject({
                data_collection: "deny",
                zdr: true,
                allow_fallbacks: false,
                max_price: { prompt: 0, completion: 0, request: 0, image: 0, audio: 0 },
              });
              expect(payload.models).toBeUndefined();
              expect(["openrouter/free", "xingchen-agi/xing4.0-29b-a4b:free"]).toContain(
                payload.model,
              );
              const chunk = {
                id: "fixture-reply",
                object: "chat.completion.chunk",
                created: 1,
                model: "xingchen-agi/xing4.0-29b-a4b:free",
              };
              if (nextToolCall) {
                const call = nextToolCall;
                nextToolCall = null;
                const frames = [
                  {
                    ...chunk,
                    choices: [
                      {
                        index: 0,
                        delta: {
                          role: "assistant",
                          tool_calls: [
                            {
                              index: 0,
                              id: call.id,
                              type: "function",
                              function: {
                                name: "tasks_lists_create",
                                arguments: JSON.stringify({
                                  title: "Native approval fixture",
                                  connectionId: call.connectionId,
                                }),
                              },
                            },
                          ],
                        },
                        finish_reason: null,
                      },
                    ],
                  },
                  { ...chunk, choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }] },
                ];
                return new Response(
                  frames.map((f) => "data: " + JSON.stringify(f) + "\n\n").join("") +
                    "data: [DONE]\n\n",
                  { headers: { "Content-Type": "text/event-stream" } },
                );
              }
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
            if (sessionId === delayedSessionCheck) {
              delayedSessionCheck = undefined;
              await new Promise((resolve) => setTimeout(resolve, 1250));
            }
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
              user_id: sessionUsers.get(sessionId) ?? owner,
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
  for (const file of [
    "0001_initial_schema.sql",
    "0002_oauth_states_discovery.sql",
    "0003_rate_limit_counters.sql",
    "0004_mcp_signal_counters.sql",
    "0005_tool_grants.sql",
    "0007_chat_threads.sql",
  ])
    for (const sql of splitMigrationStatements(loadMigrationSql(file))) await db.prepare(sql).run();
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
  result?: unknown;
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
async function cookieFor(sessionId: string, user = owner) {
  const jwt = await signTestToken(
    {
      sub: user,
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
  const result = await done;
  expect(result, JSON.stringify(result)).not.toHaveProperty("error", true);
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

it("persists native write approval over reconnect and never dispatches an unapproved or forged action", async () => {
  const db = await mf.getD1Database("DB");
  const repo = new HostedRepository(db as unknown as D1Database);
  const tenant = await repo.bootstrapTenant(owner);
  const connection = await repo.createConnection(
    tenant.id,
    "fixture-google",
    "fixture@example.test",
    "Fixture",
    "[]",
  );
  await repo.saveToolGrants(tenant.id, connection.id, [
    {
      service: "tasks",
      toolName: "tasks_lists_create",
      enabled: true,
      allowDestructive: false,
      allowBillable: false,
    },
  ]);
  const sessionCookie = await cookieFor("session-tool-approvals");
  const meta = await mf.dispatchFetch(origin + "/api/chat/session", {
    headers: { Cookie: sessionCookie },
  });
  const path =
    origin + "/agents/chat/" + ((await meta.json()) as { conversation: string }).conversation;
  let socket = await open(path, sessionCookie);
  nextToolCall = { id: "native-write-approval", connectionId: connection.id };
  await turn(socket, "native-approval-turn", [
    {
      id: "native-user",
      role: "user",
      parts: [{ type: "text", text: "Create the fixture list, after asking approval." }],
    },
  ]);
  const messages = await history(path, sessionCookie);
  expect(JSON.stringify(messages)).toContain("approval-requested");
  expect(JSON.stringify(messages)).toContain("native-write-approval");
  expect((await modelRpc(socket, "selectModel", ["xing-free"])).success).toBe(true);
  expect(await history(path, sessionCookie)).toEqual(messages);

  const before = await db
    .prepare("SELECT COUNT(*) AS count FROM hosted_audit_events WHERE action = 'mcp.tools.call'")
    .first<{ count: number }>();
  socket.close();
  socket = await open(path, sessionCookie);
  expect((await modelRpc(socket, "modelSettings")).result).toMatchObject({
    selectedKey: "xing-free",
  });
  expect(JSON.stringify(await history(path, sessionCookie))).toContain("approval-requested");
  // Native approval denial must settle the original tool, not execute it.
  socket.send(
    JSON.stringify({
      type: "cf_agent_tool_approval",
      toolCallId: "native-write-approval",
      approved: false,
      autoContinue: true,
    }),
  );
  await expect
    .poll(async () => JSON.stringify(await history(path, sessionCookie)))
    .toContain("output-denied");
  const after = await db
    .prepare("SELECT COUNT(*) AS count FROM hosted_audit_events WHERE action = 'mcp.tools.call'")
    .first<{ count: number }>();
  expect(after).toEqual(before);
  // Approval dispatches exactly once into the shared MCP path. This fixture has
  // no Google credentials/runner, so the result must honestly report unavailable.
  nextToolCall = { id: "native-write-approved", connectionId: connection.id };
  await turn(socket, "native-approved-turn", [
    {
      id: "native-approved-user",
      role: "user",
      parts: [{ type: "text", text: "Ask before creating another fixture list." }],
    },
  ]);
  expect(JSON.stringify(await history(path, sessionCookie))).toContain("approval-requested");
  // Fresh authorization can outlast Vitest's one-second default on shared CI.
  // Exercise that delay while retaining the existing five-second native wait budget.
  delayedSessionCheck = "session-tool-approvals";
  socket.send(
    JSON.stringify({
      type: "cf_agent_tool_approval",
      toolCallId: "native-write-approved",
      approved: true,
      autoContinue: true,
    }),
  );
  await expect
    .poll(
      async () =>
        (await db
          .prepare(
            "SELECT COUNT(*) AS count FROM hosted_audit_events WHERE action = 'mcp.tools.call'",
          )
          .first<{ count: number }>())!.count,
      { timeout: 5000 },
    )
    .toBe(before!.count + 1);
  expect(delayedSessionCheck).toBeUndefined();
  await expect
    .poll(async () => JSON.stringify(await history(path, sessionCookie)))
    .toContain("isError");
  // A made-up browser approval has no durable server-originated call to authorize.
  const close = closed(socket);
  socket.send(
    JSON.stringify({
      type: "cf_agent_tool_approval",
      toolCallId: "forged-call",
      approved: true,
      autoContinue: true,
    }),
  );
  expect(await close).toBe(4400);
});

it("isolates native transcripts/approvals across threads and purges a deleted thread", async () => {
  const sessionCookie = await cookieFor("session-native-threads");
  const headers = { Cookie: sessionCookie, Origin: origin, "Content-Type": "application/json" };
  const paths: string[] = [];
  const ids: string[] = [];
  for (const title of ["Native thread A", "Native thread B"]) {
    const created = await mf.dispatchFetch(origin + "/api/chat/threads", {
      method: "POST",
      headers,
      body: JSON.stringify({ title }),
    });
    expect(created.status).toBe(201);
    const { thread } = (await created.json()) as { thread: { id: string } };
    ids.push(thread.id);
    const meta = await mf.dispatchFetch(origin + "/api/chat/session?thread=" + thread.id, {
      headers,
    });
    const data = (await meta.json()) as { conversation: string; draftKey: string };
    expect(data.draftKey).toBe(data.conversation);
    paths.push(origin + "/agents/chat/" + data.conversation);
    expect(await history(paths.at(-1)!, sessionCookie)).toEqual([]);
  }
  expect(paths[0]).not.toBe(paths[1]);
  const a = await open(paths[0], sessionCookie),
    b = await open(paths[1], sessionCookie);
  await turn(a, "thread-a-message", [
    { id: "thread-a-user", role: "user", parts: [{ type: "text", text: "Only in thread A" }] },
  ]);
  expect(JSON.stringify(await history(paths[0], sessionCookie))).toContain("Only in thread A");
  expect(await history(paths[1], sessionCookie)).toEqual([]);
  const db = await mf.getD1Database("DB");
  const repo = new HostedRepository(db as unknown as D1Database);
  const tenant = await repo.bootstrapTenant(owner);
  const connection = (await repo.listConnections(tenant.id))[0]!;
  nextToolCall = { id: "thread-a-only-approval", connectionId: connection.id };
  await turn(a, "thread-a-proposal", [
    {
      id: "thread-a-write",
      role: "user",
      parts: [{ type: "text", text: "Propose the fixture action in thread A" }],
    },
  ]);
  expect(JSON.stringify(await history(paths[0], sessionCookie))).toContain("approval-requested");
  const bClosed = closed(b);
  b.send(
    JSON.stringify({
      type: "cf_agent_tool_approval",
      toolCallId: "thread-a-only-approval",
      approved: true,
      autoContinue: true,
    }),
  );
  expect(await bClosed).toBe(4400);
  expect(JSON.stringify(await history(paths[0], sessionCookie))).toContain("approval-requested");
  const deleted = await mf.dispatchFetch(origin + "/api/chat/threads/" + ids[0], {
    method: "DELETE",
    headers,
  });
  expect(deleted.status).toBe(204);
  expect((await mf.dispatchFetch(paths[0] + "/get-messages", { headers })).status).toBe(404);
  expect(
    (await mf.dispatchFetch(origin + "/api/chat/session?thread=" + ids[0], { headers })).status,
  ).toBe(404);
  expect(
    (await mf.dispatchFetch(paths[1] + "/__thread-delete", { method: "DELETE", headers })).status,
  ).toBe(404);
  expect((await mf.dispatchFetch(paths[1] + "/__legacy-budget", { headers })).status).toBe(404);
  expect(await history(paths[1], sessionCookie)).toEqual([]);
  const legacy = await mf.dispatchFetch(origin + "/api/chat/session?thread=original", { headers });
  const data = (await legacy.json()) as { conversation: string };
  expect(
    JSON.stringify(await history(origin + "/agents/chat/" + data.conversation, sessionCookie)),
  ).toContain("B-only native fixture message");
});

it("admits two independent native cohort tenants and rejects foreign or revoked live sessions", async () => {
  const tenantA = await new HostedRepository(
    (await mf.getD1Database("DB")) as unknown as D1Database,
  ).bootstrapTenant(cohortA);
  const tenantB = await new HostedRepository(
    (await mf.getD1Database("DB")) as unknown as D1Database,
  ).bootstrapTenant(cohortB);
  expect(tenantA.id).not.toBe(tenantB.id);
  sessionUsers.set("sess_native_cohort_a", cohortA);
  sessionUsers.set("sess_native_cohort_b", cohortB);

  const cookieA = await cookieFor("sess_native_cohort_a", cohortA);
  const cookieB = await cookieFor("sess_native_cohort_b", cohortB);
  const metaA = await mf.dispatchFetch(origin + "/api/chat/session?thread=original", {
    headers: { Cookie: cookieA },
  });
  const metaB = await mf.dispatchFetch(origin + "/api/chat/session?thread=original", {
    headers: { Cookie: cookieB },
  });
  expect(metaA.status).toBe(200);
  expect(metaB.status).toBe(200);
  const pathA =
    origin + "/agents/chat/" + ((await metaA.json()) as { conversation: string }).conversation;
  const pathB =
    origin + "/agents/chat/" + ((await metaB.json()) as { conversation: string }).conversation;
  expect(pathA).not.toBe(pathB);
  expect(await history(pathA, cookieA)).toEqual([]);
  expect(await history(pathB, cookieB)).toEqual([]);

  const socketA = await open(pathA, cookieA);
  const socketB = await open(pathB, cookieB);
  await turn(socketA, "cohort-a-turn", [
    { id: "cohort-a-user", role: "user", parts: [{ type: "text", text: "Only cohort A" }] },
  ]);
  await turn(socketB, "cohort-b-turn", [
    { id: "cohort-b-user", role: "user", parts: [{ type: "text", text: "Only cohort B" }] },
  ]);
  expect(JSON.stringify(await history(pathA, cookieA))).toContain("Only cohort A");
  expect(
    (await mf.dispatchFetch(pathA + "/get-messages", { headers: { Cookie: cookieB } })).status,
  ).toBe(404);
  expect(JSON.stringify(await history(pathB, cookieB))).toContain("Only cohort B");
  expect(
    (await mf.dispatchFetch(pathB + "/get-messages", { headers: { Cookie: cookieA } })).status,
  ).toBe(404);
  expect(
    (await mf.dispatchFetch(pathB + "/get-messages", { headers: { Cookie: cookieA } })).status,
  ).toBe(404);
  expect(
    (
      await mf.dispatchFetch(origin + "/api/chat/session?thread=" + tenantB.id, {
        headers: { Cookie: cookieA },
      })
    ).status,
  ).toBe(404);

  const checksBeforeForeign = sessionChecks;
  const foreignCookie = await cookieFor("sess_native_foreign", "user_not_invited");
  expect(
    (
      await mf.dispatchFetch(origin + "/api/chat/session", {
        headers: { Cookie: foreignCookie },
      })
    ).status,
  ).toBe(403);
  expect(sessionChecks).toBe(checksBeforeForeign);

  revoked.add("sess_native_cohort_a");
  const closedA = closed(socketA);
  socketA.send(
    JSON.stringify({
      type: "cf_agent_use_chat_request",
      id: "revoked-a-turn",
      init: {
        method: "POST",
        body: JSON.stringify({
          messages: [{ id: "revoked-a-user", role: "user", parts: [{ type: "text", text: "No" }] }],
          trigger: "submit-message",
        }),
      },
    }),
  );
  expect(await closedA).toBe(4401);
  revoked.delete("sess_native_cohort_a");
  expect(
    (await mf.dispatchFetch(pathA + "/get-messages", { headers: { Cookie: cookieA } })).status,
  ).toBe(200);
  await open(pathA, cookieA).then((socket) => socket.close());

  sessionUsers.delete("sess_native_cohort_a");
  sessionUsers.delete("sess_native_cohort_b");
});

async function modelRpc(socket: Socket, method: string, args: unknown[] = []) {
  const id = "model-rpc-" + crypto.randomUUID();
  const response = frame(socket, (value) => value.id === id);
  socket.send(JSON.stringify({ type: "rpc", id, method, args }));
  return response;
}

it("persists allowed model selection per thread, rejects paid/forged choices, and keeps native history over reconnect", async () => {
  const sessionCookie = await cookieFor("sess-native-models");
  const headers = { Cookie: sessionCookie, Origin: origin, "Content-Type": "application/json" };
  const paths: string[] = [];
  for (const title of ["Model thread A", "Model thread B"]) {
    const created = await mf.dispatchFetch(origin + "/api/chat/threads", {
      method: "POST",
      headers,
      body: JSON.stringify({ title }),
    });
    expect(created.status).toBe(201);
    const { thread } = (await created.json()) as { thread: { id: string } };
    const meta = await mf.dispatchFetch(origin + "/api/chat/session?thread=" + thread.id, {
      headers,
    });
    paths.push(
      origin + "/agents/chat/" + ((await meta.json()) as { conversation: string }).conversation,
    );
  }
  const a = await open(paths[0], sessionCookie),
    b = await open(paths[1], sessionCookie);
  const initial = await modelRpc(a, "modelSettings");
  expect(initial.success).toBe(true);
  expect(initial.result).toMatchObject({ selectedKey: "free-router", model: "openrouter/free" });
  const before = providerCalls;
  for (const choice of [
    "gpt41-mini",
    "openai/gpt-4.1-mini",
    "forged-model",
    "minimax-free",
    "nemotron-free",
  ]) {
    expect((await modelRpc(a, "selectModel", [choice])).success).toBe(false);
  }
  expect(providerCalls).toBe(before);
  const selected = await modelRpc(a, "selectModel", ["xing-free"]);
  expect(selected.success).toBe(true);
  expect(selected.result).toMatchObject({
    selectedKey: "xing-free",
    model: "xingchen-agi/xing4.0-29b-a4b:free",
  });
  expect((await modelRpc(b, "modelSettings")).result).toMatchObject({ selectedKey: "free-router" });
  await turn(a, "model-selected-turn", [
    {
      id: "model-user",
      role: "user",
      parts: [{ type: "text", text: "Keep this history while switching models" }],
    },
  ]);
  const saved = await history(paths[0], sessionCookie);
  expect(saved).toHaveLength(2);
  a.close();
  const reconnected = await open(paths[0], sessionCookie);
  const persisted = await modelRpc(reconnected, "modelSettings");
  expect(persisted.result).toMatchObject({
    selectedKey: "xing-free",
    lastModelId: "xingchen-agi/xing4.0-29b-a4b:free",
  });
  expect((await modelRpc(reconnected, "selectModel", ["free-router"])).success).toBe(true);
  expect(await history(paths[0], sessionCookie)).toEqual(saved);
  revoked.add("sess-native-models");
  const ended = closed(reconnected);
  reconnected.send(
    JSON.stringify({
      type: "rpc",
      id: "revoked-model-change",
      method: "selectModel",
      args: ["free-router"],
    }),
  );
  expect(await ended).toBe(4401);
  b.close();
});

it("enforces application-wide provider request limits before dispatch without losing the sent message", async () => {
  const sessionCookie = await cookieFor("sess-native-model-limit");
  const headers = { Cookie: sessionCookie, Origin: origin, "Content-Type": "application/json" };
  const created = await mf.dispatchFetch(origin + "/api/chat/threads", {
    method: "POST",
    headers,
    body: JSON.stringify({ title: "Limited model conversation" }),
  });
  expect(created.status).toBe(201);
  const { thread } = (await created.json()) as { thread: { id: string } };
  const meta = await mf.dispatchFetch(origin + "/api/chat/session?thread=" + thread.id, {
    headers,
  });
  const path =
    origin + "/agents/chat/" + ((await meta.json()) as { conversation: string }).conversation;
  const socket = await open(path, sessionCookie);
  const db = await mf.getD1Database("DB");
  await db
    .prepare(
      `INSERT INTO hosted_rate_limit_counters(scope_kind,scope_key,period,value,updated_at) VALUES('tenant','gog-chat:global',?1,50,?2) ON CONFLICT(scope_kind,scope_key,period) DO UPDATE SET value=50`,
    )
    .bind("model-requests-day:" + Math.floor(Date.now() / 86400000), new Date().toISOString())
    .run();
  const before = providerCalls;
  const response = frame(
    socket,
    (value) =>
      value.type === "cf_agent_use_chat_response" &&
      value.id === "model-limit-turn" &&
      !!value.error,
  );
  socket.send(
    JSON.stringify({
      type: "cf_agent_use_chat_request",
      id: "model-limit-turn",
      init: {
        method: "POST",
        body: JSON.stringify({
          messages: [
            {
              id: "limited-user",
              role: "user",
              parts: [{ type: "text", text: "A saved but limited request" }],
            },
          ],
          trigger: "submit-message",
        }),
      },
    }),
  );
  expect(await response).toHaveProperty("error", true);
  expect(providerCalls).toBe(before);
  expect(JSON.stringify(await history(path, sessionCookie))).toContain(
    "A saved but limited request",
  );
  socket.close();
});
