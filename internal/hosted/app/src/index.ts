/**
 * Cloudflare Worker entrypoint for the hosted v1 Clerk-protected UI shell.
 *
 * Routes:
 * - `GET /` — Clerk sign-in/sign-up for unauthenticated visitors; product
 *   home for authenticated users.
 * - `GET /api/tenant` — protected product API; returns user-safe session
 *   status and identity only (never the internal tenant UUID).
 * - `GET /api/google/connections` — protected list of the tenant's Google
 *   connections (verified identity + discovery status, never credentials).
 * - `POST /api/google/connect` — starts the hosted Google OAuth flow; the
 *   returned authorization URL is bound to the Clerk tenant/session and the
 *   intended connection via a one-use server-side state/PKCE record.
 * - `GET /oauth/google/callback` — completes the Google OAuth flow (the
 *   canonical registered redirect URI is /oauth/google/callback).
 * - `POST /api/google/connections/:id/disconnect` — revokes + deletes the
 *   connection, its credentials, and its grants.
 * - `POST /api/google/connections/:id/refresh` — validates/refreshes the
 *   stored credential; expired/revoked grants surface a distinct outcome.
 *
 * No generic HTTP proxy, exec tool, Google Connect, runner, control-plane,
 * or MCP endpoints are exposed.
 */
import { HostedRepository } from "../../state/src/repository.js";
import { authenticate, type ClerkEnv } from "./auth.js";
import {
  buildConnectDeps,
  completeCallback,
  ConnectError,
  disconnect,
  listConnections,
  refreshConnection,
  startConnect,
  type ConnectEnv,
  type ConnectSession,
} from "./connect.js";
import {
  deriveClerkDomain,
  renderCallbackNotice,
  renderHome,
  renderSignIn,
  securityHeaders,
  type HtmlConfig,
} from "./html.js";

export interface Env extends ClerkEnv, ConnectEnv {
  CLERK_SECRET_KEY?: string;
  CLERK_PUBLISHABLE_KEY?: string;
  CLERK_AUTHORIZED_PARTIES?: string;
  CLERK_ISSUER?: string;
  CLERK_JWT_KEY?: string;
  DB: D1Database;
}

function safeError(status: number, message: string, clerkDomain = ""): Response {
  return Response.json({ error: message }, { status, headers: securityHeaders(clerkDomain) });
}

function connectErrorResponse(status: number, code: string, message: string): Response {
  return Response.json({ error: message, code }, { status, headers: securityHeaders() });
}

/**
 * True only for a real browser navigation to the Google OAuth callback.
 * Terminal Clerk failures on this route must never strand the browser on a
 * secret-bearing code/state URL with a JSON body.
 */
function isBrowserGoogleCallback(request: Request, path: string): boolean {
  return (
    path === "/oauth/google/callback" &&
    request.method === "GET" &&
    Boolean(request.headers.get("accept")?.includes("text/html"))
  );
}

/**
 * Safe human-facing HTML for a terminal Clerk failure on the browser Google
 * callback. `title` and `message` are fixed operator-defined strings only;
 * request-derived values (code, state, nonce, raw auth/provider errors) are
 * never interpolated. The page strips the callback query parameters from the
 * address bar client-side, which cannot create a redirect loop.
 */
function browserCallbackNotice(status: number, title: string, message: string): Response {
  return new Response(renderCallbackNotice(title, message), {
    status,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      ...securityHeaders(),
      "Cache-Control": "no-store",
      "Referrer-Policy": "no-referrer",
    },
  });
}

const GOOGLE_API_PATHS = [
  "/api/google/connections",
  "/api/google/connect",
  "/oauth/google/callback",
];
const GOOGLE_CONNECTION_ACTION = /^\/api\/google\/connections\/([^/]+)\/(disconnect|refresh)$/;

async function handleRoute(request: Request, env: Env, repo: HostedRepository): Promise<Response> {
  const url = new URL(request.url);
  const path = url.pathname;
  const connectionAction = GOOGLE_CONNECTION_ACTION.exec(path);

  // Only the known routes are served. Everything else is 404.
  const knownPaths = ["/", "/api/tenant", ...GOOGLE_API_PATHS];
  if (!knownPaths.includes(path) && !connectionAction) {
    return safeError(404, "Not found.");
  }

  if ((GOOGLE_API_PATHS.includes(path) || connectionAction) && request.method === "POST") {
    // Cookie-bearing requests always require a trusted same-origin Origin,
    // even if an Authorization header is also present. Bearer-only API
    // clients may omit Origin; supplied origins must still match.
    const origin = request.headers.get("origin");
    if (
      (request.headers.has("cookie") && !origin) ||
      (origin !== null &&
        (origin !== url.origin ||
          !(env.CLERK_AUTHORIZED_PARTIES ?? "")
            .split(",")
            .map((s) => s.trim())
            .includes(origin)))
    ) {
      return connectErrorResponse(403, "csrf_rejected", "This request could not be verified.");
    }
    if (request.headers.get("content-type")?.split(";")[0].trim() !== "application/json") {
      return connectErrorResponse(400, "invalid_request", "A JSON request is required.");
    }
    try {
      const body = await request.clone().json();
      if (body === null || typeof body !== "object" || Array.isArray(body))
        throw new Error("shape");
    } catch {
      return connectErrorResponse(
        400,
        "invalid_request",
        "The Google connection request was invalid.",
      );
    }
  }

  // The authorized-party allowlist comes from configuration only; it is
  // never overridden with the untrusted request origin.
  const outcome = await authenticate(request, env, repo);

  switch (outcome.kind) {
    case "authenticated": {
      const { session } = outcome;
      if (session.tenant.status !== "active") {
        if (isBrowserGoogleCallback(request, path)) {
          return browserCallbackNotice(
            403,
            "Workspace unavailable",
            "This workspace is not available. Please contact support if you believe this is an error.",
          );
        }
        return safeError(403, "Tenant access is not available.");
      }
      // Handshake completion can be signed-in while still carrying the
      // SDK's session cookie directives and redirect back to the clean URL.
      if (outcome.headers.has("location")) {
        return new Response(null, { status: 307, headers: outcome.headers });
      }
      if (path === "/api/tenant") {
        return Response.json(
          { status: session.tenant.status, userId: session.userId },
          { headers: outcome.headers },
        );
      }
      const isGoogleRoute = Boolean(connectionAction) || GOOGLE_API_PATHS.includes(path);
      if (isGoogleRoute) {
        const connectResponse = await handleGoogleRoute(
          request,
          env,
          repo,
          { userId: session.userId, sessionId: session.sessionId, tenantId: session.tenant.id },
          path,
          Boolean(connectionAction),
          url,
        );
        // Preserve any Clerk session-cookie directives from the SDK result.
        const headers = new Headers(connectResponse.headers);
        for (const [name, value] of outcome.headers.entries()) {
          if (name.toLowerCase() === "set-cookie") headers.append(name, value);
        }
        return new Response(connectResponse.body, {
          status: connectResponse.status,
          statusText: connectResponse.statusText,
          headers,
        });
      }
      const htmlConfig = buildHtmlConfig(env);
      const headers = new Headers(outcome.headers);
      headers.set("Content-Type", "text/html; charset=utf-8");
      for (const [name, value] of Object.entries(securityHeaders(htmlConfig.clerkDomain))) {
        headers.set(name, value);
      }
      return new Response(renderHome(htmlConfig), { status: 200, headers });
    }
    case "handshake":
      return new Response(null, { status: 307, headers: outcome.headers });
    case "unauthenticated": {
      if (isBrowserGoogleCallback(request, path)) {
        return browserCallbackNotice(
          401,
          "Session ended",
          "Your session has ended. Sign in again, then reconnect your Google account.",
        );
      }
      if (path === "/api/tenant" || GOOGLE_API_PATHS.includes(path) || connectionAction) {
        return safeError(401, "Authentication required.");
      }
      const htmlConfig = buildHtmlConfig(env);
      if (!htmlConfig.clerkDomain || !htmlConfig.publishableKey) {
        return safeError(503, "Authentication is not configured.");
      }
      return new Response(renderSignIn(htmlConfig), {
        status: 200,
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          ...securityHeaders(htmlConfig.clerkDomain),
        },
      });
    }
    case "error":
      if (isBrowserGoogleCallback(request, path)) {
        return browserCallbackNotice(
          outcome.status,
          outcome.status === 503 ? "Sign-in unavailable" : "Session check failed",
          outcome.status === 503
            ? "Sign-in is not available right now. Please try again later."
            : "We could not verify your sign-in session. Please try again in a moment.",
        );
      }
      return safeError(outcome.status, outcome.message);
  }
}

async function handleGoogleRoute(
  request: Request,
  env: Env,
  repo: HostedRepository,
  session: ConnectSession,
  path: string,
  isConnectionAction: boolean,
  url: URL,
): Promise<Response> {
  const browserCallback = isBrowserGoogleCallback(request, path);
  const browserResult = (code: string): Response =>
    new Response(null, {
      status: 303,
      headers: {
        Location: "/?google=" + encodeURIComponent(code),
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
      },
    });
  const depsOutcome = buildConnectDeps(env, repo);
  if (!depsOutcome.ok) {
    return browserCallback
      ? browserResult(depsOutcome.code)
      : connectErrorResponse(503, depsOutcome.code, depsOutcome.message);
  }
  const deps = depsOutcome.deps;

  try {
    if (request.method === "GET" && path === "/api/google/connections") {
      const result = await listConnections(deps, session);
      return Response.json(result, { headers: securityHeaders() });
    }
    if (request.method === "POST" && path === "/api/google/connect") {
      let body: Record<string, unknown> = {};
      try {
        const raw = await request.text();
        if (raw) {
          const parsed = JSON.parse(raw) as unknown;
          if (parsed !== null && typeof parsed === "object" && !Array.isArray(parsed)) {
            body = parsed as Record<string, unknown>;
          } else {
            throw new Error("invalid shape");
          }
        } else {
          throw new Error("missing JSON");
        }
      } catch {
        return connectErrorResponse(
          400,
          "invalid_request",
          "The Google connection request was invalid.",
        );
      }
      const result = await startConnect(deps, session, body);
      return Response.json(result, { headers: securityHeaders() });
    }
    if (request.method === "GET" && path === "/oauth/google/callback") {
      const result = await completeCallback(deps, session, url.searchParams);
      return browserCallback
        ? browserResult("connected")
        : Response.json(result, { headers: securityHeaders() });
    }
    if (isConnectionAction) {
      const match = GOOGLE_CONNECTION_ACTION.exec(path)!;
      const connectionId = decodeURIComponent(match[1] ?? "").trim();
      if (!connectionId) {
        return connectErrorResponse(
          404,
          "connection_not_found",
          "That Google connection no longer exists.",
        );
      }
      if (request.method === "POST" && match[2] === "disconnect") {
        await disconnect(deps, session, connectionId);
        return Response.json({ ok: true }, { headers: securityHeaders() });
      }
      if (request.method === "POST" && match[2] === "refresh") {
        const connection = await refreshConnection(deps, session, connectionId);
        return Response.json({ connection }, { headers: securityHeaders() });
      }
    }
    return connectErrorResponse(405, "invalid_request", "Unsupported request for this route.");
  } catch (error) {
    if (error instanceof ConnectError) {
      return browserCallback
        ? browserResult(error.code)
        : connectErrorResponse(error.status, error.code, error.message);
    }
    if (browserCallback) return browserResult("internal_error");
    return connectErrorResponse(
      500,
      "internal_error",
      "Something went wrong connecting your Google account.",
    );
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const repo = new HostedRepository(env.DB);
    try {
      return await handleRoute(request, env, repo);
    } catch {
      if (isBrowserGoogleCallback(request, new URL(request.url).pathname)) {
        return browserCallbackNotice(
          500,
          "Unexpected error",
          "Something went wrong. Please try again later.",
        );
      }
      return safeError(500, "Internal error.");
    }
  },
};

function buildHtmlConfig(env: Env): HtmlConfig {
  const publishableKey = env.CLERK_PUBLISHABLE_KEY ?? "";
  return {
    publishableKey,
    authorizedParties: [],
    clerkDomain: deriveClerkDomain(publishableKey),
  };
}
