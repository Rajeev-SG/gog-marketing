/**
 * Cloudflare Worker entrypoint for the hosted v1 Clerk-protected UI shell.
 *
 * Routes:
 * - `GET /` — Clerk sign-in/sign-up for unauthenticated visitors; product
 *   home for authenticated users.
 * - `GET /api/tenant` — protected product API; returns user-safe session
 *   status and identity only (never the internal tenant UUID).
 *
 * No generic HTTP proxy, exec tool, Google Connect, runner, control-plane,
 * or MCP endpoints are exposed.
 */
import { HostedRepository } from "../../state/src/repository.js";
import { authenticate, type ClerkEnv } from "./auth.js";
import {
  deriveClerkDomain,
  renderHome,
  renderSignIn,
  securityHeaders,
  type HtmlConfig,
} from "./html.js";

export interface Env extends ClerkEnv {
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

async function handleRoute(request: Request, env: Env, repo: HostedRepository): Promise<Response> {
  const url = new URL(request.url);
  const path = url.pathname;

  // Only the known routes are served. Everything else is 404.
  const knownPaths = ["/", "/api/tenant"];
  if (!knownPaths.includes(path)) {
    return safeError(404, "Not found.");
  }

  // The authorized-party allowlist comes from configuration only; it is
  // never overridden with the untrusted request origin.
  const outcome = await authenticate(request, env, repo);

  switch (outcome.kind) {
    case "authenticated": {
      const { session } = outcome;
      if (session.tenant.status !== "active") {
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
      if (path === "/api/tenant") {
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
      return safeError(outcome.status, outcome.message);
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const repo = new HostedRepository(env.DB);
    try {
      return await handleRoute(request, env, repo);
    } catch {
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
