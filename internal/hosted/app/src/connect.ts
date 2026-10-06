/**
 * Hosted Google account connection lifecycle (issue #62).
 *
 * Separate from Clerk identity: the Clerk session answers "who is signed in";
 * Google OAuth answers "which Google account and scopes may gog-marketing
 * use". Every callback is bound to the verified Clerk tenant AND the exact
 * Clerk session that started the flow AND the intended connection via a
 * one-use server-side state/PKCE/nonce record. The returned Google identity
 * is verified (userinfo, verified email, OIDC nonce/audience) before any
 * connection is created or replaced.
 *
 * Distinct, user-facing outcomes are represented by `ConnectErrorCode`.
 * Refresh credentials are AES-GCM encrypted at rest with key-version
 * metadata and are never part of any response, log, or audit row.
 */
import { HostedRepository } from "../../state/src/repository.js";
import type { GoogleConnection } from "../../state/src/types.js";
import {
  CredentialCipher,
  CredentialKeyError,
  parseCredentialKeys,
  type StoredGoogleCredential,
} from "./credential-crypto.js";
import {
  RemoteDiscoveryRunner,
  type DiscoveryOutcome,
  type DiscoveryRunner,
  UnavailableDiscoveryRunner,
} from "./discovery.js";
import {
  GoogleOAuthClient,
  GoogleOAuthError,
  type GoogleEndpoints,
  type GoogleTokenSet,
} from "./google-oauth.js";
import {
  HOSTED_SERVICES,
  mergeScopes,
  normalizeServices,
  UnknownServiceError,
  scopesForServices,
} from "./scopes.js";
import { WorkloadIdentityClient, validateRunnerIdentityConfig } from "./runner-auth.js";

export interface ConnectEnv {
  GOG_GOOGLE_OAUTH_CLIENT_ID?: string;
  GOG_GOOGLE_OAUTH_CLIENT_SECRET?: string;
  GOG_GOOGLE_OAUTH_REDIRECT_URI?: string;
  /** Non-secret endpoint overrides (staging/tests); must be absolute http(s) URLs. */
  GOG_GOOGLE_OAUTH_AUTH_URL?: string;
  GOG_GOOGLE_OAUTH_TOKEN_URL?: string;
  GOG_GOOGLE_OAUTH_USERINFO_URL?: string;
  GOG_GOOGLE_OAUTH_REVOKE_URL?: string;
  /** Secret: standard base64 of 32 random bytes, key_version 1. */
  GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY?: string;
  /** Optional rotation map: {"2": "<base64 32B>", ...}; encrypt with highest. */
  GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS?: string;
  /** #63 Go discovery runner (not configured until #63 lands). */
  GOG_CLOUD_RUN_SERVICE_URL?: string;
  GOG_RUNNER_WIF_PROVIDER?: string;
  GOG_RUNNER_WIF_ISSUER?: string;
  GOG_RUNNER_WIF_KEY_ID?: string;
  GOG_RUNNER_WIF_SIGNING_KEY?: string;
  GOG_RUNNER_SERVICE_ACCOUNT?: string;
  GOG_RUNNER_INVOCATION_TOKEN?: string;
  GOG_HOSTED_OAUTH_STATE_TTL_SECONDS?: string;
  /** Test-only fetch injection; never set in production. */
  __testFetch?: typeof fetch;
}

export type ConnectErrorCode =
  | "operator_config_missing"
  | "consent_denied"
  | "invalid_request"
  | "state_unknown"
  | "state_replayed"
  | "state_expired"
  | "authorization_expired"
  | "callback_wrong_tenant"
  | "callback_wrong_session"
  | "callback_wrong_connection"
  | "google_api_error"
  | "google_unavailable"
  | "google_identity_failed"
  | "identity_conflict"
  | "connection_not_found"
  | "needs_reconnect"
  | "discovery_unavailable"
  | "discovery_error"
  | "internal_error";

export const CONNECT_ERROR_MESSAGES: Record<ConnectErrorCode, string> = {
  operator_config_missing: "Google connection is not configured by the operator yet.",
  consent_denied: "Google consent was denied, so the account was not connected.",
  invalid_request: "The Google connection request was invalid.",
  state_unknown: "This Google connection attempt is no longer valid. Please start again.",
  state_replayed: "This Google connection attempt was already completed. Please start again.",
  authorization_expired: "The Google authorization attempt expired. Please start again.",
  state_expired: "The Google connection attempt expired. Please start again.",
  callback_wrong_tenant: "This Google connection attempt belongs to a different workspace.",
  callback_wrong_session:
    "This Google connection attempt was started from a different sign-in session.",
  callback_wrong_connection: "The intended Google connection no longer exists. Please start again.",
  google_api_error: "Google's service could not complete the connection.",
  google_unavailable: "Google's service is temporarily unreachable. Please try again.",
  google_identity_failed: "Google's account identity could not be verified.",
  identity_conflict: "That Google account is already connected to a different connection.",
  connection_not_found: "That Google connection no longer exists.",
  needs_reconnect: "This Google account needs to be reconnected.",
  discovery_unavailable: "The account is connected. Resource discovery is temporarily unavailable.",
  discovery_error: "The account is connected, but resource discovery failed.",
  internal_error: "Something went wrong connecting your Google account.",
};

/** Map a connect error to the audit-safe stable error code allowlist. */
export function auditSafeError(code: ConnectErrorCode): string {
  switch (code) {
    case "consent_denied":
    case "callback_wrong_tenant":
    case "identity_conflict":
      return "permission_denied";
    case "invalid_request":
    case "state_unknown":
    case "state_replayed":
    case "state_expired":
      return "invalid_request";
    case "callback_wrong_session":
    case "google_identity_failed":
      return "unauthenticated";
    case "callback_wrong_connection":
    case "connection_not_found":
      return "not_found";
    case "needs_reconnect":
      return "needs_reconnect";
    case "operator_config_missing":
    case "google_api_error":
    case "google_unavailable":
    case "discovery_unavailable":
    case "discovery_error":
      return "provider_unavailable";
    default:
      return "internal_error";
  }
}

export class ConnectError extends Error {
  readonly code: ConnectErrorCode;
  readonly status: number;

  constructor(code: ConnectErrorCode, status: number) {
    super(CONNECT_ERROR_MESSAGES[code]);
    this.name = "ConnectError";
    this.code = code;
    this.status = status;
  }
}

export interface ConnectSession {
  /** Verified Clerk user ID. */
  userId: string;
  /** Verified Clerk session ID (`sid`) from the same server-verified session. */
  sessionId: string;
  /** Internal tenant ID resolved server-side from the Clerk identity. */
  tenantId: string;
}

export interface DiscoveryView {
  status: DiscoveryOutcome["status"];
  detail: string;
  resourceCount: number;
  checkedAt: string | null;
}

/** User-safe connection projection: no tenant ID, no credential material. */
export interface ConnectionView {
  id: string;
  email: string;
  displayName: string;
  grantedScopes: string[];
  status: GoogleConnection["status"];
  lastError: string;
  lastValidatedAt: string | null;
  discovery: DiscoveryView;
  createdAt: string;
  updatedAt: string;
}

export interface ConnectDeps {
  repo: HostedRepository;
  oauth: GoogleOAuthClient;
  cipher: CredentialCipher;
  discovery: DiscoveryRunner;
  redirectUri: string;
  stateTtlSeconds: number;
  now?: () => Date;
}

export type DepsOutcome =
  | { ok: true; deps: ConnectDeps }
  | { ok: false; code: "operator_config_missing"; message: string };

function absoluteUrl(value: string): URL | null {
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" && url.protocol !== "http:") return null;
    return url;
  } catch {
    return null;
  }
}

export function buildConnectDeps(env: ConnectEnv, repo: HostedRepository): DepsOutcome {
  const clientId = (env.GOG_GOOGLE_OAUTH_CLIENT_ID ?? "").trim();
  const clientSecret = (env.GOG_GOOGLE_OAUTH_CLIENT_SECRET ?? "").trim();
  const redirectRaw = (env.GOG_GOOGLE_OAUTH_REDIRECT_URI ?? "").trim();
  if (!clientId || !clientSecret || !redirectRaw) {
    return {
      ok: false,
      code: "operator_config_missing",
      message: CONNECT_ERROR_MESSAGES.operator_config_missing,
    };
  }
  const redirectUri = absoluteUrl(redirectRaw);
  if (!redirectUri) {
    return {
      ok: false,
      code: "operator_config_missing",
      message: CONNECT_ERROR_MESSAGES.operator_config_missing,
    };
  }

  const endpointNames = [
    ["GOG_GOOGLE_OAUTH_AUTH_URL", env.GOG_GOOGLE_OAUTH_AUTH_URL],
    ["GOG_GOOGLE_OAUTH_TOKEN_URL", env.GOG_GOOGLE_OAUTH_TOKEN_URL],
    ["GOG_GOOGLE_OAUTH_USERINFO_URL", env.GOG_GOOGLE_OAUTH_USERINFO_URL],
    ["GOG_GOOGLE_OAUTH_REVOKE_URL", env.GOG_GOOGLE_OAUTH_REVOKE_URL],
  ] as const;
  const endpoints: Record<string, string> = {};
  for (const [name, value] of endpointNames) {
    if (!value) continue;
    const parsed = absoluteUrl(value.trim());
    if (!parsed) {
      return {
        ok: false,
        code: "operator_config_missing",
        message: CONNECT_ERROR_MESSAGES.operator_config_missing,
      };
    }
    endpoints[
      {
        GOG_GOOGLE_OAUTH_AUTH_URL: "authUrl",
        GOG_GOOGLE_OAUTH_TOKEN_URL: "tokenUrl",
        GOG_GOOGLE_OAUTH_USERINFO_URL: "userinfoUrl",
        GOG_GOOGLE_OAUTH_REVOKE_URL: "revokeUrl",
      }[name]
    ] = parsed.toString().replace(/\/$/, "");
  }

  let cipher: CredentialCipher;
  try {
    cipher = new CredentialCipher(
      parseCredentialKeys(
        env.GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY,
        env.GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS,
      ),
    );
  } catch {
    return {
      ok: false,
      code: "operator_config_missing",
      message: CONNECT_ERROR_MESSAGES.operator_config_missing,
    };
  }

  const runnerUrlRaw = (env.GOG_CLOUD_RUN_SERVICE_URL ?? "").trim();
  const runnerToken = (env.GOG_RUNNER_INVOCATION_TOKEN ?? "").trim();
  const runnerConfigured = Boolean(
    runnerUrlRaw ||
    runnerToken ||
    env.GOG_RUNNER_WIF_PROVIDER ||
    env.GOG_RUNNER_WIF_ISSUER ||
    env.GOG_RUNNER_WIF_KEY_ID ||
    env.GOG_RUNNER_WIF_SIGNING_KEY ||
    env.GOG_RUNNER_SERVICE_ACCOUNT,
  );
  let discovery: DiscoveryRunner = new UnavailableDiscoveryRunner();
  if (runnerConfigured) {
    try {
      const runnerUrl = new URL(runnerUrlRaw);
      if (runnerUrl.protocol !== "https:" || runnerUrl.pathname !== "/v1/execute") {
        throw new Error("runner URL is not configured");
      }
      if (new TextEncoder().encode(runnerToken).length < 32) {
        throw new Error("invocation secret is not configured");
      }
      const identityConfig = {
        provider: (env.GOG_RUNNER_WIF_PROVIDER ?? "").trim(),
        issuer: (env.GOG_RUNNER_WIF_ISSUER ?? "").trim(),
        keyId: (env.GOG_RUNNER_WIF_KEY_ID ?? "").trim(),
        signingKey: (env.GOG_RUNNER_WIF_SIGNING_KEY ?? "").trim(),
        serviceAccount: (env.GOG_RUNNER_SERVICE_ACCOUNT ?? "").trim(),
      };
      validateRunnerIdentityConfig(identityConfig);
      const fetchImpl = env.__testFetch ?? fetch.bind(globalThis);
      discovery = new RemoteDiscoveryRunner({
        url: runnerUrl,
        invocationToken: runnerToken,
        identity: new WorkloadIdentityClient({ config: identityConfig, fetchImpl }),
        fetchImpl,
      });
    } catch {
      // A partially or incorrectly configured runner stays unavailable; it
      // must never fall back to static bearer authentication.
      discovery = new UnavailableDiscoveryRunner();
    }
  }

  const oauth = new GoogleOAuthClient({
    clientId,
    clientSecret,
    endpoints: endpoints as unknown as Partial<GoogleEndpoints>,
    fetchImpl: env.__testFetch,
  });

  const ttlRaw = Number(env.GOG_HOSTED_OAUTH_STATE_TTL_SECONDS ?? 600);
  const stateTtlSeconds = Number.isFinite(ttlRaw) && ttlRaw > 0 ? Math.floor(ttlRaw) : 600;
  return {
    ok: true,
    deps: { repo, oauth, cipher, discovery, redirectUri: redirectUri.toString(), stateTtlSeconds },
  };
}

// ─── Random token helpers ───────────────────────────────────────────────────

function randomToken(bytes: number): string {
  const raw = crypto.getRandomValues(new Uint8Array(bytes));
  let binary = "";
  for (const byte of raw) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

async function stateHash(state: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(state));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// ─── Views ──────────────────────────────────────────────────────────────────

function viewFor(connection: GoogleConnection): ConnectionView {
  let grantedScopes: string[] = [];
  try {
    const parsed = JSON.parse(connection.grantedScopesJson);
    if (Array.isArray(parsed)) grantedScopes = parsed.map(String);
  } catch {
    grantedScopes = [];
  }
  return {
    id: connection.id,
    email: connection.email,
    displayName: connection.displayName,
    grantedScopes,
    status: connection.status,
    lastError: connection.lastError,
    lastValidatedAt: connection.lastValidatedAt,
    discovery: {
      status: (connection.discoveryState || "unavailable") as DiscoveryView["status"],
      detail: connection.discoveryDetail,
      resourceCount: 0,
      checkedAt: connection.discoveryCheckedAt,
    },
    createdAt: connection.createdAt,
    updatedAt: connection.updatedAt,
  };
}

async function audit(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
  action: string,
  result: "allow" | "deny" | "error" | "info",
  detail: Record<string, unknown>,
): Promise<void> {
  try {
    await deps.repo.appendAudit({
      id: crypto.randomUUID(),
      tenantId: session.tenantId,
      connectionId,
      actorClerkUserId: session.userId,
      action,
      result,
      detailJson: JSON.stringify(detail),
      latencyMs: null,
    });
  } catch {
    // Audit must never break the user-visible flow or leak details.
  }
}

// ─── Start ──────────────────────────────────────────────────────────────────

export interface StartResult {
  authorizationUrl: string;
  connectionId: string;
}

export async function startConnect(
  deps: ConnectDeps,
  session: ConnectSession,
  body: { connectionId?: unknown; services?: unknown },
): Promise<StartResult> {
  let services: string[];
  try {
    services = normalizeServices(body.services);
  } catch (error) {
    if (error instanceof UnknownServiceError) {
      throw new ConnectError("invalid_request", 400);
    }
    throw new ConnectError("invalid_request", 400);
  }
  scopesForServices(services); // validates; identity scopes always included

  const intent: "connect" | "reconnect" = body.connectionId ? "reconnect" : "connect";
  let connection: GoogleConnection | null = null;
  if (intent === "reconnect") {
    const id = String(body.connectionId ?? "").trim();
    if (!id) throw new ConnectError("invalid_request", 400);
    connection = await deps.repo.getConnection(session.tenantId, id);
    if (!connection) throw new ConnectError("connection_not_found", 404);
  } else {
    // Pending placeholder; the verified Google identity replaces it only
    // after the callback identity check. Placeholder subjects are unique so
    // multiple concurrent attempts never collide.
    connection = await deps.repo.createConnection(
      session.tenantId,
      `pending-${crypto.randomUUID()}`,
      "",
      "",
      "[]",
    );
  }

  // Reconnect retains previously consented hosted capabilities, even when
  // the UI submits only the stable connection handle.
  if (intent === "reconnect") {
    const previous = JSON.parse(connection.grantedScopesJson) as string[];
    for (const [service, info] of Object.entries(HOSTED_SERVICES)) {
      if (info.scopes.every((scope) => previous.includes(scope)) && !services.includes(service)) {
        services.push(service);
      }
    }
  }
  const scopes = scopesForServices(services);
  const state = randomToken(32);
  const codeVerifier = randomToken(48);
  const nonce = randomToken(24);
  const now = deps.now?.() ?? new Date();
  const expiresAt = new Date(now.getTime() + deps.stateTtlSeconds * 1000).toISOString();

  await deps.repo.createOAuthState({
    stateHash: await stateHash(state),
    tenantId: session.tenantId,
    connectionId: connection.id,
    clerkUserId: session.userId,
    clerkSessionId: session.sessionId,
    intent,
    codeVerifier,
    nonce,
    redirectUri: deps.redirectUri,
    scopes,
    services,
    expiresAt,
  });

  const authorizationUrl = await deps.oauth.authorizationURL({
    state,
    codeVerifier,
    nonce,
    scopes,
    redirectUri: deps.redirectUri,
    forceConsent: true,
  });

  await audit(deps, session, connection.id, "google.connect.start", "info", {
    operation: intent,
    service: services[0] ?? "",
    error: "none",
  });

  return { authorizationUrl, connectionId: connection.id };
}

// ─── Callback ───────────────────────────────────────────────────────────────

export interface CallbackResult {
  connection: ConnectionView;
  discovery: DiscoveryView;
}

function failureToConnectError(error: unknown): ConnectError {
  if (error instanceof GoogleOAuthError) {
    switch (error.kind) {
      case "consent_denied":
        return new ConnectError("consent_denied", 403);
      case "invalid_grant":
        return new ConnectError("authorization_expired", 409);
      case "api_error":
        return new ConnectError("google_api_error", 502);
      case "network":
        return new ConnectError("google_unavailable", 502);
      case "identity_failed":
      case "nonce_mismatch":
        return new ConnectError("google_identity_failed", 403);
      case "invalid_request":
      case "config_missing":
        return new ConnectError("operator_config_missing", 503);
      default:
        return new ConnectError("internal_error", 500);
    }
  }
  if (error instanceof CredentialKeyError) {
    return new ConnectError("operator_config_missing", 503);
  }
  return new ConnectError("internal_error", 500);
}

async function markConnectionFailed(
  deps: ConnectDeps,
  session: ConnectSession,
  connection: GoogleConnection,
  _code: ConnectErrorCode,
): Promise<void> {
  // A pending placeholder never carries identity or credentials: delete it.
  const isStub = connection.googleSubject.startsWith("pending-");
  if (isStub) {
    await deps.repo.deleteConnection(session.tenantId, connection.id);
    return;
  }
  // Failed authorization of a new code does not establish that the existing
  // stored grant is dead. Preserve its health, identity, ciphertext and grants.
}

export async function completeCallback(
  deps: ConnectDeps,
  session: ConnectSession,
  query: URLSearchParams,
): Promise<CallbackResult> {
  const errorParam = (query.get("error") ?? "").trim();
  const code = (query.get("code") ?? "").trim();
  const state = (query.get("state") ?? "").trim();

  if (errorParam) {
    if (!state) throw new ConnectError("consent_denied", 403);
    const take = await deps.repo.takeOAuthState(
      session.tenantId,
      await stateHash(state),
      session.sessionId,
    );
    if (take.kind === "taken") {
      const connection = await deps.repo.getConnection(session.tenantId, take.state.connectionId);
      if (connection) {
        await markConnectionFailed(deps, session, connection, "consent_denied");
        await audit(deps, session, connection.id, "google.connect.consent_denied", "deny", {
          operation: take.state.intent,
          error: "permission_denied",
        });
      }
    }
    throw new ConnectError("consent_denied", 403);
  }

  if (!code || !state) throw new ConnectError("invalid_request", 400);

  const take = await deps.repo.takeOAuthState(
    session.tenantId,
    await stateHash(state),
    session.sessionId,
  );
  switch (take.kind) {
    case "unknown":
      throw new ConnectError("state_unknown", 400);
    case "replayed":
      throw new ConnectError("state_replayed", 409);
    case "expired":
      throw new ConnectError("state_expired", 410);
    case "wrong_tenant":
      throw new ConnectError("callback_wrong_tenant", 403);
    case "wrong_session":
      throw new ConnectError("callback_wrong_session", 403);
  }

  const bound = take.state;
  const connection = await deps.repo.getConnection(session.tenantId, bound.connectionId);
  if (!connection) {
    throw new ConnectError("callback_wrong_connection", 403);
  }

  let token: GoogleTokenSet;
  let identity: Awaited<ReturnType<GoogleOAuthClient["identity"]>>;
  try {
    token = await deps.oauth.exchangeCode({
      code,
      codeVerifier: bound.codeVerifier,
      nonce: bound.nonce,
      redirectUri: bound.redirectUri,
    });
    identity = await deps.oauth.identity(token.accessToken);
  } catch (error) {
    const connectError = failureToConnectError(error);
    await markConnectionFailed(deps, session, connection, connectError.code);
    await audit(deps, session, connection.id, "google.connect.failure", "error", {
      operation: bound.intent,
      error: auditSafeError(connectError.code),
    });
    throw connectError;
  }

  if (bound.intent === "reconnect") {
    // Reconnecting may only restore the SAME account on this connection. A
    // different Google account is rejected; the connection (and any other
    // connection) is never replaced.
    const sameSubject = connection.googleSubject && connection.googleSubject === identity.subject;
    if (!sameSubject) {
      await audit(deps, session, connection.id, "google.connect.failure", "deny", {
        operation: "reconnect",
        error: "permission_denied",
      });
      throw new ConnectError("identity_conflict", 409);
    }
  }

  // The verified identity must not collide with another connection in this
  // tenant. Multiple accounts coexist; a duplicate identity is rejected and
  // the other connection is left untouched.
  const conflict = await deps.repo.findIdentityConflict(
    session.tenantId,
    connection.id,
    identity.subject,
    identity.email,
  );
  if (conflict) {
    await markConnectionFailed(deps, session, connection, "identity_conflict");
    await audit(deps, session, connection.id, "google.connect.failure", "deny", {
      operation: bound.intent,
      error: "permission_denied",
    });
    throw new ConnectError("identity_conflict", 409);
  }

  // Persist encrypted credentials (AES-GCM, versioned, AAD-bound) first; the
  // identity and scope update happens only after the credential write.
  const payload: StoredGoogleCredential = {
    access_token: token.accessToken,
    refresh_token: token.refreshToken,
    token_type: token.tokenType,
    expiry: token.expiry,
    granted_scopes: token.grantedScopes,
  };
  try {
    const encrypted = await deps.cipher.encrypt(session.tenantId, connection.id, payload);
    await deps.repo.upsertCredential({
      tenantId: session.tenantId,
      connectionId: connection.id,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
  } catch (error) {
    const connectError = failureToConnectError(error);
    await markConnectionFailed(deps, session, connection, connectError.code);
    throw connectError;
  }

  const previousScopes = (() => {
    try {
      const parsed = JSON.parse(connection.grantedScopesJson);
      return Array.isArray(parsed) ? parsed.map(String) : [];
    } catch {
      return [];
    }
  })();
  const mergedScopes = mergeScopes(previousScopes, token.grantedScopes);
  await deps.repo.updateConnectionIdentity(session.tenantId, connection.id, {
    googleSubject: identity.subject,
    email: identity.email,
    displayName: identity.displayName || identity.email,
    grantedScopesJson: JSON.stringify(mergedScopes),
  });
  await deps.repo.updateConnectionStatus(session.tenantId, connection.id, "active", "");
  await audit(deps, session, connection.id, "google.connect.complete", "allow", {
    operation: bound.intent,
    service: bound.services[0] ?? "",
    error: "none",
  });

  // Discovery is best-effort and distinct: unavailable/failed discovery
  // never erases or expands existing grants.
  const discovery = await runDiscovery(
    deps,
    session,
    connection.id,
    bound.services,
    token.accessToken,
  );

  const updated = await deps.repo.getConnection(session.tenantId, connection.id);
  return {
    connection: viewFor(updated ?? connection),
    discovery,
  };
}

// ─── Discovery application (safe semantics) ─────────────────────────────────

async function runDiscovery(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
  services: string[],
  accessToken: string,
): Promise<DiscoveryView> {
  const persisted = await deps.repo.getConnection(session.tenantId, connectionId);
  const outcome: DiscoveryOutcome =
    persisted?.email && persisted.googleSubject
      ? await discoverWithPersistedIdentity(deps, session, connectionId, services, accessToken, {
          email: persisted.email,
          subject: persisted.googleSubject,
        })
      : {
          status: "unavailable",
          detail: "connection_identity_missing",
          resources: [],
          statuses: {},
        };

  const checkedAt = new Date(deps.now?.() ?? new Date()).toISOString();
  if (outcome.status === "ok" || outcome.status === "empty") {
    // Preserve user enable/disable choices for known resources; new
    // resources default to disabled (least privilege, no auto-expansion).
    const existing = await deps.repo.listResourceGrants(session.tenantId, connectionId);
    const preserved = new Map(existing.map((g) => [`${g.service}:${g.resourceId}`, g.enabled]));
    for (const resource of outcome.resources) {
      const enabled = preserved.get(`${resource.service}:${resource.resourceId}`) ?? false;
      await deps.repo.upsertResourceGrant(
        session.tenantId,
        connectionId,
        resource.service,
        resource.resourceId,
        resource.resourceType,
        resource.displayName,
        enabled,
        JSON.stringify(resource.metadata),
      );
    }
  }

  const persistedState =
    outcome.status === "ok" ? "ok" : outcome.status === "empty" ? "empty" : outcome.status;
  await deps.repo.updateConnectionDiscovery(
    session.tenantId,
    connectionId,
    persistedState,
    outcome.detail,
    checkedAt,
  );

  if (outcome.status === "unavailable") {
    await audit(deps, session, connectionId, "google.discovery", "info", {
      error: "provider_unavailable",
      operation: "discovery",
    });
  } else if (outcome.status === "error") {
    await audit(deps, session, connectionId, "google.discovery", "error", {
      error: "provider_unavailable",
      operation: "discovery",
    });
  }

  return {
    status: outcome.status,
    detail: outcome.detail,
    resourceCount: outcome.resources.length,
    checkedAt,
  };
}

async function discoverWithPersistedIdentity(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
  services: string[],
  accessToken: string,
  identity: { email: string; subject: string },
): Promise<DiscoveryOutcome> {
  try {
    return await deps.discovery.discover({
      tenantId: session.tenantId,
      connectionId,
      services,
      accessToken,
      googleEmail: identity.email,
      googleSubject: identity.subject,
    });
  } catch {
    return { status: "error", detail: "runner_unreachable", resources: [], statuses: {} };
  }
}

// ─── Disconnect / refresh / list ────────────────────────────────────────────

export async function disconnect(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
): Promise<{ ok: true }> {
  const connection = await deps.repo.getConnection(session.tenantId, connectionId);
  if (!connection) throw new ConnectError("connection_not_found", 404);

  // Best-effort Google revocation. The local credential is removed
  // regardless so a disconnect never leaves usable material behind.
  try {
    const stored = await deps.repo.getCredential(session.tenantId, connectionId);
    if (stored) {
      const payload = await deps.cipher.decrypt(session.tenantId, connectionId, stored);
      const revokeToken = payload.refresh_token || payload.access_token;
      if (revokeToken) await deps.oauth.revoke(revokeToken);
    }
  } catch {
    // Revocation failure does not block local teardown.
  }
  await deps.repo.deleteConnection(session.tenantId, connectionId);
  await audit(deps, session, connectionId, "google.disconnect", "info", {
    error: "none",
    operation: "disconnect",
  });
  return { ok: true };
}

export async function refreshConnection(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
): Promise<ConnectionView> {
  const connection = await deps.repo.getConnection(session.tenantId, connectionId);
  if (!connection) throw new ConnectError("connection_not_found", 404);

  const stored = await deps.repo.getCredential(session.tenantId, connectionId);
  if (!stored) {
    await deps.repo.updateConnectionStatus(
      session.tenantId,
      connectionId,
      "needs_reconnect",
      CONNECT_ERROR_MESSAGES.needs_reconnect,
    );
    throw new ConnectError("needs_reconnect", 409);
  }

  let payload: StoredGoogleCredential;
  try {
    payload = await deps.cipher.decrypt(session.tenantId, connectionId, stored);
  } catch (error) {
    throw failureToConnectError(error);
  }

  let token: GoogleTokenSet;
  try {
    token = await deps.oauth.refreshTokenSet({
      refreshToken: payload.refresh_token || payload.access_token,
      previous: {
        accessToken: payload.access_token,
        refreshToken: payload.refresh_token,
        tokenType: payload.token_type,
        expiry: payload.expiry,
        grantedScopes: payload.granted_scopes,
        idToken: "",
      },
    });
  } catch (error) {
    if (error instanceof GoogleOAuthError && error.kind === "invalid_grant") {
      // Expired/revoked grant: drop the dead credential, keep the connection
      // row and its grants (user configuration is preserved), and surface the
      // distinct reconnect outcome.
      await deps.repo.deleteCredential(session.tenantId, connectionId);
      await deps.repo.updateConnectionStatus(
        session.tenantId,
        connectionId,
        "needs_reconnect",
        CONNECT_ERROR_MESSAGES.needs_reconnect,
      );
      await audit(deps, session, connectionId, "google.connect.refresh", "error", {
        error: "needs_reconnect",
        operation: "refresh",
      });
      throw new ConnectError("needs_reconnect", 409);
    }
    const connectError = failureToConnectError(error);
    await audit(deps, session, connectionId, "google.connect.refresh", "error", {
      error: auditSafeError(connectError.code),
      operation: "refresh",
    });
    throw connectError;
  }

  const encrypted = await deps.cipher.encrypt(session.tenantId, connectionId, {
    access_token: token.accessToken,
    refresh_token: token.refreshToken || payload.refresh_token,
    token_type: token.tokenType,
    expiry: token.expiry,
    granted_scopes: token.grantedScopes.length ? token.grantedScopes : payload.granted_scopes,
  });
  await deps.repo.upsertCredential({
    tenantId: session.tenantId,
    connectionId,
    ciphertext: encrypted.ciphertext,
    nonce: encrypted.nonce,
    keyVersion: Number(encrypted.keyVersion),
  });

  const previousScopes = payload.granted_scopes;
  await deps.repo.updateConnectionIdentity(session.tenantId, connectionId, {
    googleSubject: connection.googleSubject,
    email: connection.email,
    displayName: connection.displayName,
    grantedScopesJson: JSON.stringify(mergeScopes(previousScopes, token.grantedScopes)),
  });
  await deps.repo.updateConnectionStatus(session.tenantId, connectionId, "active", "");
  await audit(deps, session, connectionId, "google.connect.refresh", "allow", {
    error: "none",
    operation: "refresh",
  });

  const updated = await deps.repo.getConnection(session.tenantId, connectionId);
  return viewFor(updated ?? connection);
}

export async function listConnections(
  deps: ConnectDeps,
  session: ConnectSession,
): Promise<{ connections: ConnectionView[] }> {
  const connections = await deps.repo.listConnections(session.tenantId);
  return { connections: connections.map(viewFor) };
}
