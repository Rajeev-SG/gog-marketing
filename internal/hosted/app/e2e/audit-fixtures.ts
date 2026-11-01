import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import type { Page, WebSocketRoute } from "@playwright/test";

const here = fileURLToPath(new URL(".", import.meta.url));
const appRoot = join(here, "..");
const dist = join(appRoot, "dist", "client");
const axeSource = readFileSync(join(appRoot, "node_modules", "axe-core", "axe.min.js"), "utf8");

export const CLERK_DOMAIN = "fixture.clerk.accounts.dev";

type HtmlModule = {
  renderHome: (config: {
    publishableKey: string;
    authorizedParties: string[];
    clerkDomain: string;
    mcpOrigin: string;
  }) => string;
};
let htmlModule: Promise<HtmlModule> | undefined;
function loadHtml(): Promise<HtmlModule> {
  htmlModule ??= build({
    entryPoints: [join(appRoot, "src", "html.ts")],
    bundle: true,
    format: "esm",
    platform: "neutral",
    target: "es2022",
    write: false,
  }).then(
    (bundle) =>
      import(
        `data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}`
      ) as Promise<HtmlModule>,
  );
  return htmlModule;
}

/** Synthetic connections: one populated account and one connected account with no assets. */
export function fixtureConnections() {
  const resource = (service: string, label: string, id: string, enabled: boolean) => ({
    service,
    serviceLabel: label,
    resourceId: id,
    displayName: `${label} asset ${id}`,
    resourceType: "property",
    parent: "Fixture parent",
    enabled,
  });
  return {
    connections: [
      {
        id: "conn-populated",
        email: "owner@example.com",
        status: "active",
        discovery: { status: "complete" },
        services: ["Google Analytics 4", "Google Tag Manager", "Drive (selected files)"],
        resources: [
          resource("analytics", "Google Analytics 4", "1", true),
          resource("analytics", "Google Analytics 4", "2", true),
          resource("tagmanager", "Google Tag Manager", "3", false),
          resource("drive", "Drive (selected files)", "4", true),
        ],
      },
      {
        id: "conn-empty",
        email: "empty@example.com",
        status: "active",
        discovery: { status: "empty" },
        services: ["Google Analytics 4", "Drive (selected files)"],
        resources: [],
      },
    ],
  };
}

const CLERK_STUB = `window.__internal_ClerkUICtor = function(){};
window.Clerk = { loaded: true, session: { id: 'fixture' }, load: async function(){}, mountUserButton: function(el){ el.innerHTML = '<button type="button" aria-label="Open user menu" class="fixture-avatar">R</button>'; }, signOut: async function(){}, addListener: function(fn){ fn({ session: { id: 'fixture' } }); return function(){}; } };`;

/** Serve the real server-rendered Home with a stubbed Clerk and mocked Google APIs. */
export async function openHome(
  page: Page,
  connections = fixtureConnections(),
  options: { toolsEnabled?: boolean } = {},
) {
  const { renderHome } = await loadHtml();
  const html = renderHome({
    publishableKey: "pk_test_fixture",
    authorizedParties: ["https://test.example.com"],
    clerkDomain: CLERK_DOMAIN,
    mcpOrigin: "https://test.example.com",
  });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.hostname === CLERK_DOMAIN) {
      return route.fulfill({
        contentType: "application/javascript",
        body: url.pathname.includes("clerk-js") ? CLERK_STUB : "",
      });
    }
    if (url.pathname === "/") return route.fulfill({ contentType: "text/html", body: html });
    if (url.pathname === "/api/google/resources") return route.fulfill({ json: connections });
    if (url.pathname === "/api/google/tools")
      return route.fulfill({
        json: {
          tools: [
            {
              name: "analytics_run_report",
              service: "analytics",
              description: "Run a report.",
              write: false,
              enabled: options.toolsEnabled ?? false,
              allowDestructive: false,
            },
          ],
        },
      });
    return route.abort();
  });
  await page.goto("http://127.0.0.1:8799/");
  await page.locator("#workspace-content").waitFor({ state: "visible" });
  return errors;
}

export const MODEL_SETTINGS = {
  selectedKey: "free-router",
  model: "openrouter/free",
  models: [
    {
      key: "free-router",
      label: "Automatic · Free",
      description: "OpenRouter chooses an eligible free model with tool support.",
      free: true,
      enabled: true,
    },
    {
      key: "xing-free",
      label: "Xing 4.0 · Free",
      description: "A tool-capable free alternative.",
      free: true,
      enabled: true,
    },
    {
      key: "minimax-free",
      label: "MiniMax M3.1 Flash · Free",
      description: "A free tool-capable alternative.",
      free: true,
      enabled: false,
      disabledReason:
        "No zero-data-retention provider is currently listed for this free model. Choose Automatic or Xing Free.",
    },
  ],
  limits: { globalRequestsPerDay: 50, tenantRequestsPerDay: 100, tenantTokensPerDay: 2_000_000 },
  usage: {
    tenantRequestsToday: 100,
    tenantTokensToday: 400_000,
    globalRequestsToday: 50,
    resetsAt: "2026-11-02T00:00:00.000Z",
  },
};

export const LIMIT_ERROR =
  "You've reached the chat usage limit. Per-minute limits clear within a minute; daily allowances reset at 00:00 UTC. Open Model details to see today's usage. No paid model is used automatically.";

/**
 * Serve the built chat client with a native-protocol websocket fixture.
 * `savedMessages` stands in for the persisted conversation, `failNext` makes the
 * next model turn end with a stream error, like the live capacity failure.
 */
export async function openChat(
  page: Page,
  options: { savedMessages?: unknown[]; failWith?: string; reply?: string } = {},
) {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    Object.assign(window, {
      Clerk: {
        loaded: true,
        session: { id: "fixture-session" },
        addListener(fn: (p: { session: { id: string } }) => void) {
          fn({ session: { id: "fixture-session" } });
          return () => {};
        },
        async signOut() {},
      },
    });
  });
  const now = new Date().toISOString();
  const thread = {
    id: "00000000-0000-4000-8000-00000000000a",
    title: "Campaign brief for spring launch",
    conversation: "a".repeat(64),
    createdAt: now,
    updatedAt: now,
  };
  const sockets: WebSocketRoute[] = [];
  await page.routeWebSocket("**/agents/**", (socket) => {
    sockets.push(socket);
    socket.onMessage((raw) => {
      const frame = JSON.parse(String(raw)) as {
        type: string;
        id: string;
        method?: string;
      };
      if (frame.type === "cf_agent_stream_resume_request") {
        socket.send(
          JSON.stringify({
            type: "cf_agent_stream_resume_none",
            probeId: (frame as { probeId?: string }).probeId,
          }),
        );
        return;
      }
      if (frame.type === "rpc") {
        const result =
          frame.method === "modelSettings" || frame.method === "selectModel"
            ? MODEL_SETTINGS
            : null;
        socket.send(
          JSON.stringify({ type: "rpc", id: frame.id, success: true, result, done: true }),
        );
        return;
      }
      if (frame.type === "cf_agent_use_chat_request") {
        const send = (body: unknown, extra: Record<string, unknown> = {}) =>
          socket.send(
            JSON.stringify({
              type: "cf_agent_use_chat_response",
              id: frame.id,
              body: JSON.stringify(body),
              done: false,
              ...extra,
            }),
          );
        send({ type: "start", messageId: "assistant-" + frame.id });
        if (options.failWith) {
          // Mirrors AIChatAgent: an `error` chunk is broadcast as an error frame.
          socket.send(
            JSON.stringify({
              type: "cf_agent_use_chat_response",
              id: frame.id,
              body: options.failWith,
              done: false,
              error: true,
            }),
          );
          return;
        }
        send({ type: "text-start", id: "t1" });
        send({ type: "text-delta", id: "t1", delta: options.reply ?? "Here is a short reply." });
        send({ type: "text-end", id: "t1" });
        send({ type: "finish" });
        socket.send(
          JSON.stringify({
            type: "cf_agent_use_chat_response",
            id: frame.id,
            body: "",
            done: true,
          }),
        );
      }
    });
  });
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/chat")
      return route.fulfill({
        contentType: "text/html",
        body: readFileSync(join(dist, "chat.html")),
      });
    if (url.pathname.startsWith("/assets/"))
      return route.fulfill({
        contentType: url.pathname.endsWith(".css") ? "text/css" : "application/javascript",
        body: readFileSync(join(dist, url.pathname.slice(1))),
      });
    if (url.pathname === "/api/chat/threads")
      return route.fulfill({ json: { threads: [thread], pendingDeletions: [] } });
    if (url.pathname === "/api/chat/session")
      return route.fulfill({
        json: {
          thread,
          conversation: thread.conversation,
          draftKey: thread.conversation,
          model: "openrouter/free",
          expiresAt: Date.now() + 3_600_000,
        },
      });
    if (url.pathname.endsWith("/get-messages"))
      return route.fulfill({ json: options.savedMessages ?? [] });
    return route.abort();
  });
  await page.goto("http://127.0.0.1:8799/chat");
  await page.getByRole("textbox", { name: "Message", exact: true }).waitFor();
  return { errors, sockets };
}

/** Run axe-core (WCAG 2.2 A/AA rules) on the current page. */
export async function axeViolations(page: Page) {
  await page.addScriptTag({ content: axeSource });
  return page.evaluate(async () => {
    const result = await (
      window as unknown as {
        axe: {
          run: (
            context: Document,
            options: object,
          ) => Promise<{
            violations: {
              id: string;
              impact: string;
              nodes: { target: string[]; any: { message: string }[] }[];
            }[];
          }>;
        };
      }
    ).axe.run(document, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] },
    });
    return result.violations.map(
      (v) =>
        `${v.id} (${v.impact}): ` +
        v.nodes.map((n) => `${n.target.join(" ")} — ${n.any[0]?.message ?? ""}`).join(" | "),
    );
  });
}

export async function horizontalOverflow(page: Page) {
  return page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
}
