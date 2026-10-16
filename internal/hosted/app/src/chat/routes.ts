import { ChatThreads, publicThread, threadTitle, validThreadId } from "./threads.js";
/** Same-origin chat routes. Existing product, Google and MCP routes fall through unchanged. */
import type { HostedRepository } from "../../../state/src/repository.js";
import {
  clerkScriptTags,
  deriveClerkDomain,
  renderSignIn,
  securityHeaders,
  type HtmlConfig,
} from "../html.js";
import { authenticateChatOwner, chatModelIdentifier, type ChatEnv } from "./access.js";

function privateHeaders(env: ChatEnv, request: Request, initial?: Headers): Headers {
  const domain = deriveClerkDomain(env.CLERK_PUBLISHABLE_KEY ?? "");
  const headers = new Headers(initial);
  for (const [name, value] of Object.entries(securityHeaders(domain))) headers.set(name, value);
  const url = new URL(request.url);
  const socketOrigin = (url.protocol === "https:" ? "wss://" : "ws://") + url.host;
  headers.set(
    "Content-Security-Policy",
    headers
      .get("Content-Security-Policy")!
      .replace(
        /connect-src ([^;]*)/,
        (_all, sources: string) => "connect-src " + sources + " " + socketOrigin,
      ),
  );
  headers.set("Cache-Control", "no-store");
  return headers;
}

function config(env: ChatEnv): HtmlConfig {
  return {
    publishableKey: env.CLERK_PUBLISHABLE_KEY ?? "",
    clerkDomain: deriveClerkDomain(env.CLERK_PUBLISHABLE_KEY ?? ""),
    authorizedParties: (env.CLERK_AUTHORIZED_PARTIES ?? "")
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean),
    mcpOrigin: "",
  };
}

function errorResponse(status: number, message: string, page: boolean, headers: Headers): Response {
  if (!page) return Response.json({ error: message }, { status, headers });
  // Messages are fixed application/authentication copy, never raw provider exceptions.
  const escaped = message
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll('"', "&quot;");
  headers.set("Content-Type", "text/html; charset=utf-8");
  return new Response(
    '<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Chat — gog-marketing</title></head><body><main><h1>Chat unavailable</h1><p>' +
      escaped +
      '</p><a href="/">Return to Home</a></main></body></html>',
    { status, headers },
  );
}

export async function handleChat(
  request: Request,
  env: ChatEnv,
  repo: HostedRepository,
): Promise<Response | null> {
  const url = new URL(request.url);
  const page = url.pathname === "/chat" || url.pathname === "/chat/";
  const metadata = url.pathname === "/api/chat/session";
  // Authenticate known chat paths before comparing their caller-supplied name.
  const collection = url.pathname === "/api/chat/threads";
  const threadPath = /^\/api\/chat\/threads\/([^/]+)$/.exec(url.pathname);
  const agent = /^\/agents\/chat\/([^/]+)(\/get-messages)?$/.exec(url.pathname);
  if (!page && !metadata && !agent && !collection && !threadPath) {
    if (url.pathname.startsWith("/agents/") || url.pathname.startsWith("/api/chat/"))
      return errorResponse(404, "Not found.", false, privateHeaders(env, request));
    return null;
  }
  const headers = privateHeaders(env, request);
  if (request.method !== "GET" && !collection && !threadPath) {
    headers.set("Allow", "GET");
    return errorResponse(405, "Method not allowed.", page, headers);
  }
  if (agent) {
    const upgrade = request.headers.get("Upgrade")?.toLowerCase() === "websocket";
    const origin = request.headers.get("Origin");
    const trusted = origin === url.origin && config(env).authorizedParties.includes(origin);
    if ((upgrade || origin !== null) && !trusted) {
      return errorResponse(403, "This request could not be verified.", false, headers);
    }
  }
  const outcome = await authenticateChatOwner(request, env, repo);
  if (
    outcome.kind === "handshake" ||
    (outcome.kind === "authenticated" && outcome.headers.has("location"))
  ) {
    const redirectHeaders = privateHeaders(env, request, outcome.headers);
    return new Response(null, { status: 307, headers: redirectHeaders });
  }
  if (outcome.kind === "error")
    return errorResponse(outcome.status, outcome.message, page, headers);
  if (outcome.kind === "unauthenticated") {
    if (!page) return errorResponse(401, "Sign in to continue.", false, headers);
    headers.set("Content-Type", "text/html; charset=utf-8");
    const selected = url.searchParams.get("thread");
    const returnTo = selected && validThreadId(selected) ? "/chat?thread=" + selected : "/chat";
    return new Response(
      renderSignIn(config(env)).replace(
        "fallbackRedirectUrl: '/'",
        "fallbackRedirectUrl: '" + returnTo + "'",
      ),
      { headers },
    );
  }
  const authenticatedHeaders = privateHeaders(env, request, outcome.headers);
  const registry = new ChatThreads(repo, outcome.session.tenant.id);
  if (collection || threadPath) {
    if (
      request.method !== "GET" &&
      (request.headers.get("Origin") !== url.origin ||
        !config(env).authorizedParties.includes(url.origin))
    )
      return errorResponse(403, "This request could not be verified.", false, authenticatedHeaders);
    if (
      request.method !== "GET" &&
      !request.headers.get("Content-Type")?.toLowerCase().startsWith("application/json")
    )
      return errorResponse(415, "JSON is required.", false, authenticatedHeaders);
    const id = threadPath?.[1];
    if (id && !validThreadId(id))
      return errorResponse(404, "Conversation not found.", false, authenticatedHeaders);
    if (collection && request.method === "GET")
      return Response.json(
        {
          threads: (await registry.list()).map(publicThread),
          pendingDeletions: await registry.pendingDeletions(),
        },
        { headers: authenticatedHeaders },
      );
    if (id && request.method === "DELETE") {
      const record = await registry.get(id, true);
      if (!record)
        return errorResponse(404, "Conversation not found.", false, authenticatedHeaders);
      await registry.tombstone(id);
      if (!record.purged) {
        if (!env.Chat)
          return errorResponse(
            503,
            "Deletion is pending. Retry to finish removing this conversation.",
            false,
            authenticatedHeaders,
          );
        const { routeAgentRequest } = await import("agents");
        const purge = await routeAgentRequest(
          new Request(new URL("/agents/chat/" + record.conversation + "/__thread-delete", url), {
            method: "DELETE",
            headers: { "x-gog-thread-tenant": outcome.session.tenant.id },
          }),
          env,
        );
        if (!purge?.ok)
          return errorResponse(
            503,
            "Deletion is pending. Retry to finish removing this conversation.",
            false,
            authenticatedHeaders,
          );
        await registry.markPurged(id);
      }
      return new Response(null, { status: 204, headers: authenticatedHeaders });
    }
    if ((collection && request.method === "POST") || (id && request.method === "PATCH")) {
      if (!request.headers.get("Content-Type")?.toLowerCase().startsWith("application/json"))
        return errorResponse(415, "JSON is required.", false, authenticatedHeaders);
      let title: string;
      try {
        const body = (await request.json()) as Record<string, unknown>;
        if (!body || Array.isArray(body) || Object.keys(body).some((k) => k !== "title"))
          throw new Error();
        title = threadTitle(body.title ?? (collection ? "New chat" : null));
      } catch {
        return errorResponse(
          400,
          "Use a conversation name of 1–100 characters.",
          false,
          authenticatedHeaders,
        );
      }
      const record = collection ? await registry.create(title) : await registry.rename(id!, title);
      if (!record)
        return errorResponse(
          collection ? 429 : 404,
          collection
            ? "Conversation limit reached (100). Delete a conversation first."
            : "Conversation not found.",
          false,
          authenticatedHeaders,
        );
      return Response.json(
        { thread: publicThread(record) },
        { status: collection ? 201 : 200, headers: authenticatedHeaders },
      );
    }
    return errorResponse(405, "Method not allowed.", false, authenticatedHeaders);
  }
  if (metadata) {
    const record = await registry.resolve(url.searchParams.get("thread"));
    if (!record) return errorResponse(404, "Conversation not found.", false, authenticatedHeaders);
    return Response.json(
      {
        model: chatModelIdentifier(env),
        conversation: record.conversation,
        draftKey: record.conversation,
        thread: publicThread(record),
        expiresAt: outcome.session.expiresAt,
      },
      { headers: authenticatedHeaders },
    );
  }
  if (agent) {
    await registry.ensureOriginal();
    if (!(await registry.byConversation(agent[1])))
      return errorResponse(404, "Conversation not found.", false, authenticatedHeaders);
    if (!env.Chat) return errorResponse(503, "Chat is not available.", false, authenticatedHeaders);
    const { routeAgentRequest } = await import("agents");
    const response = await routeAgentRequest(request, env);
    if (!response) return errorResponse(404, "Not found.", false, authenticatedHeaders);
    // Upgraded sockets cannot be cached. Ordinary native history responses are explicitly private.
    if (response.status === 101) return response;
    const historyHeaders = new Headers(response.headers);
    historyHeaders.set("Cache-Control", "no-store");
    return new Response(response.body, { status: response.status, headers: historyHeaders });
  }
  if (!env.ASSETS) return errorResponse(503, "Chat is not available.", true, authenticatedHeaders);
  // Do not forward conditional or compression headers: this private HTML is rewritten per request.
  const asset = await env.ASSETS.fetch(
    new Request(new URL("/chat.html", url), { headers: { Accept: "text/html" } }),
  );
  if (!asset.ok) return errorResponse(503, "Chat is not available.", true, authenticatedHeaders);
  const bootstrap =
    clerkScriptTags(config(env)) +
    '<script>window.addEventListener("load",async function(){await window.Clerk.load({ui:{ClerkUI:window.__internal_ClerkUICtor}});});</script>';
  const html = (await asset.text()).replace("</head>", bootstrap + "</head>");
  authenticatedHeaders.set("Content-Type", "text/html; charset=utf-8");
  return new Response(html, { headers: authenticatedHeaders });
}
