/**
 * Clerk server-side authentication for the hosted Worker.
 *
 * Uses the real @clerk/backend SDK (`createClerkClient` + `authenticateRequest`)
 * — not a custom JWT verifier. The SDK verifies the session JWT signature
 * (via JWKS or a local `jwtKey`), checks audience/authorized-party claims,
 * and exposes `requestState.toAuth()` only after successful verification.
 *
 * NOTE: `verifyJwt` in @clerk/backend 3.22 does NOT assert the `iss` claim,
 * so after the SDK's real signature verification succeeds we explicitly
 * compare `toAuth().sessionClaims.iss` against the configured Clerk issuer.
 * The issuer itself is validated against the publishable key's Frontend API
 * domain, and the authorized-party allowlist must be an explicit, non-empty
 * configuration — it is never overridden with the untrusted request origin.
 *
 * The verified Clerk user ID is the ONLY identity input. Tenant IDs, user IDs,
 * model identifiers, and similar fields are never read from request
 * body/query/header — they are always derived server-side from the verified
 * Clerk identity.
 */
import { createClerkClient, type ClerkClient } from "@clerk/backend";
import { verifyMachineAuthToken } from "@clerk/backend/internal";
import { decodeJwt } from "@clerk/backend/jwt";
import { HostedRepository } from "../../state/src/repository.js";
import { deriveClerkDomain } from "./html.js";
import type { Tenant } from "../../state/src/types.js";

export interface AuthConfig {
  secretKey: string;
  publishableKey: string;
  authorizedParties: string[];
  issuer?: string;
  audience?: string;
  /** PEM public key for networkless JWT verification (tests and local dev). */
  jwtKey?: string;
  /** Override the Backend API base URL (tests only). */
  apiUrl?: string;
}

export interface AuthSession {
  userId: string;
  /** Verified Clerk session ID (`sid` claim) used to bind OAuth flows. */
  sessionId: string;
  tenant: Tenant;
}

export type AuthOutcome =
  | { kind: "authenticated"; session: AuthSession; headers: Headers }
  | { kind: "handshake"; headers: Headers }
  | { kind: "unauthenticated" }
  | { kind: "error"; status: number; message: string };

export type BearerAuthOutcome =
  | { kind: "authenticated"; session: AuthSession }
  | { kind: "unauthenticated"; reason?: BearerAuthFailureReason }
  | { kind: "error"; status: number; message: string };

export type BearerAuthFailureReason = "invalid_token" | "opaque_token_unsupported";

export interface ClerkEnv {
  CLERK_SECRET_KEY?: string;
  CLERK_PUBLISHABLE_KEY?: string;
  CLERK_AUTHORIZED_PARTIES?: string;
  CLERK_ISSUER?: string;
  CLERK_JWT_KEY?: string;
  DB: D1Database;
}

function buildClient(config: AuthConfig): ClerkClient {
  return createClerkClient({
    secretKey: config.secretKey,
    publishableKey: config.publishableKey,
    jwtKey: config.jwtKey,
    apiUrl: config.apiUrl,
    telemetry: { disabled: true },
  });
}

export function buildOptions(config: AuthConfig): Record<string, unknown> {
  const opts: Record<string, unknown> = {
    authorizedParties: config.authorizedParties,
  };
  if (config.audience) opts.audience = config.audience;
  return opts;
}

/** Strictly extract one RFC 6750 bearer token without accepting query tokens. */
export function bearerToken(request: Request): string | null {
  const value = request.headers.get("authorization");
  if (!value) return null;
  const match = /^Bearer[ \t]+([A-Za-z0-9._~+/-]+=*)$/.exec(value.trim());
  return match?.[1] ?? null;
}

/**
 * Verify a Clerk OAuth JWT bearer token with Clerk's OAuth token verifier and
 * resolve its verified `sub` to the authoritative D1 tenant. Unlike browser
 * session authentication, OAuth `azp` is a client identifier rather than an
 * origin, so the browser authorized-party allowlist is deliberately not
 * applied here.
 */
export async function authenticateBearer(
  request: Request,
  env: ClerkEnv,
  repo: HostedRepository,
  audience: string,
): Promise<BearerAuthOutcome> {
  const token = bearerToken(request);
  if (!token) return { kind: "unauthenticated" };

  const secretKey = env.CLERK_SECRET_KEY ?? "";
  const publishableKey = env.CLERK_PUBLISHABLE_KEY ?? "";
  const issuer = validateIssuer(env.CLERK_ISSUER ?? "", publishableKey);
  if (!secretKey || !publishableKey || !issuer || !audience) {
    return {
      kind: "error",
      status: 503,
      message: "Authentication is not configured.",
    };
  }

  if (token.startsWith("oat_")) {
    return { kind: "unauthenticated", reason: "opaque_token_unsupported" };
  }

  let subject: string;
  let issuerClaim: unknown;
  let audienceClaim: unknown;
  try {
    const result = await verifyMachineAuthToken(token, {
      secretKey,
      jwtKey: env.CLERK_JWT_KEY,
      audience,
    });
    if (result.errors || result.tokenType !== "oauth_token") {
      return { kind: "unauthenticated" };
    }
    const decoded = decodeJwt(token);
    subject = result.data.subject;
    issuerClaim = decoded.payload.iss;
    audienceClaim = "aud" in result.data ? result.data.aud : undefined;
  } catch {
    return { kind: "unauthenticated" };
  }
  if (!subject || issuerClaim !== issuer || !audienceMatches(audienceClaim, audience)) {
    return { kind: "unauthenticated" };
  }
  try {
    const tenant = await resolveTenant(repo, subject);
    return {
      kind: "authenticated",
      session: {
        userId: subject,
        sessionId: `oauth:${subject}`,
        tenant,
      },
    };
  } catch {
    return {
      kind: "error",
      status: 502,
      message: "Authentication service is unavailable.",
    };
  }
}

function audienceMatches(value: unknown, expected: string): boolean {
  return Array.isArray(value) ? value.includes(expected) : value === expected;
}

/**
 * Resolve a verified Clerk identity to an internal tenant.
 * The Clerk user ID is used directly from `requestState.toAuth().userId`;
 * no other identity source is accepted.
 */
export async function resolveTenant(repo: HostedRepository, clerkUserId: string): Promise<Tenant> {
  return repo.bootstrapTenant(clerkUserId);
}

/**
 * Authenticate an incoming Worker request. Returns an `AuthOutcome` the
 * caller acts on:
 * - `authenticated`: verified Clerk session; `session.userId` and the stable
 *   internal tenant are resolved server-side via D1.
 * - `handshake`: the caller must return a 307 response carrying the
 *   SDK-provided headers (Clerk session cookie bootstrapping).
 * - `unauthenticated`: the caller must serve the sign-in page or return 401.
 * - `error`: the caller must return a safe HTTP error with no secret/token echo.
 */
export async function authenticate(
  request: Request,
  env: ClerkEnv,
  repo: HostedRepository,
  config?: Partial<AuthConfig>,
): Promise<AuthOutcome> {
  const secretKey = config?.secretKey ?? env.CLERK_SECRET_KEY ?? "";
  const publishableKey = config?.publishableKey ?? env.CLERK_PUBLISHABLE_KEY ?? "";
  const authorizedPartiesRaw =
    config?.authorizedParties ?? parseAuthorizedParties(env.CLERK_AUTHORIZED_PARTIES);
  const issuer = config?.issuer ?? env.CLERK_ISSUER ?? "";

  if (!secretKey || !publishableKey) {
    return {
      kind: "error",
      status: 503,
      message: "Authentication is not configured.",
    };
  }

  // Fail closed on missing or malformed auth configuration: the issuer must
  // be an explicit https URL on the publishable key's Frontend API domain,
  // and the authorized-party allowlist must be non-empty.
  const validatedIssuer = validateIssuer(issuer, publishableKey);
  if (!validatedIssuer || !authorizedPartiesRaw.length) {
    return {
      kind: "error",
      status: 503,
      message: "Authentication is not configured.",
    };
  }

  const fullConfig: AuthConfig = {
    secretKey,
    publishableKey,
    authorizedParties: authorizedPartiesRaw,
    issuer: validatedIssuer,
    audience: config?.audience,
    jwtKey: config?.jwtKey ?? env.CLERK_JWT_KEY,
    apiUrl: config?.apiUrl,
  };

  const clerk = buildClient(fullConfig);
  const options = buildOptions(fullConfig);

  try {
    const requestState = await clerk.authenticateRequest(request, options as never);

    if (requestState.isAuthenticated) {
      const auth = requestState.toAuth() as {
        userId?: string;
        sessionClaims?: { iss?: unknown; sid?: unknown };
      };
      const claims = auth.sessionClaims;
      const tokenIssuer = typeof claims?.iss === "string" ? claims.iss : "";
      // @clerk/backend 3.22 `verifyJwt` does not enforce `iss`; enforce it
      // explicitly against the validated configured issuer, fail closed.
      if (tokenIssuer !== validatedIssuer) {
        return { kind: "unauthenticated" };
      }
      if (typeof auth.userId !== "string") {
        return { kind: "unauthenticated" };
      }
      // OAuth lifecycle binding requires the verified Clerk session ID. Real
      // Clerk sessions always carry `sid`; a missing value fails closed.
      const sessionId = typeof claims?.sid === "string" ? claims.sid : "";
      if (!sessionId) {
        return { kind: "unauthenticated" };
      }
      const userId = auth.userId;
      const tenant = await resolveTenant(repo, userId);
      return {
        kind: "authenticated",
        session: { userId, sessionId, tenant },
        headers: requestState.headers,
      };
    }

    if (requestState.status === "handshake" || requestState.headers.has("location")) {
      return { kind: "handshake", headers: requestState.headers };
    }

    return { kind: "unauthenticated" };
  } catch {
    // Fail closed: never expose raw Clerk errors, tokens, or config values.
    return {
      kind: "error",
      status: 502,
      message: "Authentication service is unavailable.",
    };
  }
}

/**
 * Validate the configured Clerk issuer: absolute https URL, no path,
 * and its host must equal the publishable key's Frontend API domain.
 * Returns null for any missing/malformed/mismatched configuration.
 */
export function validateIssuer(issuer: string, publishableKey: string): string | null {
  try {
    const url = new URL(issuer);
    if (url.protocol !== "https:") return null;
    if (url.pathname !== "/" && url.pathname !== "") return null;
    const expectedDomain = deriveClerkDomain(publishableKey);
    if (!expectedDomain || url.host !== expectedDomain) return null;
    return `${url.protocol}//${url.host}`;
  } catch {
    return null;
  }
}

function parseAuthorizedParties(raw?: string): string[] {
  if (!raw) return [];
  const parties = raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  // Malformed entries fail closed: every authorized party must be an
  // absolute https origin.
  for (const party of parties) {
    try {
      const url = new URL(party);
      if (url.protocol !== "https:" || url.pathname !== "/") {
        return [];
      }
    } catch {
      return [];
    }
  }
  return parties;
}
