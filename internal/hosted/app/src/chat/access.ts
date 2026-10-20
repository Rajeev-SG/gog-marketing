/**
 * Chat authentication gate for the hosted Worker.
 *
 * Reuses the existing Clerk authenticate() for verified browser session
 * identity + tenant resolution, then enforces:
 * - the operator-configured chat owner (CHAT_OWNER_USER_ID) exactly;
 * - an active internal tenant;
 * - a live Clerk session (Backend API sessions.getSession), not merely a
 *   valid JWT: status, owner user, and expiry are re-checked against Clerk.
 *
 * Clerk Backend API failures fail closed and are sanitised: raw Clerk
 * errors, tokens, and configuration values are never returned, logged, or
 * persisted. Session IDs and user identity (no JWTs, tokens, or internal
 * tenant UUIDs) are the only values ever exposed to callers.
 *
 * Conversation names are server-derived opaque SHA-256 digests of the
 * internal tenant id, so clients never learn the raw tenant identifier.
 */
import { defaultChatModelKey, selectChatModel, type ChatModelEnv } from "./models.js";
import type { McpEnv } from "../mcp.js";
import { createClerkClient } from "@clerk/backend";
import { HostedRepository } from "../../../state/src/repository.js";
import type { Tenant } from "../../../state/src/types.js";
import { authenticate, type AuthOutcome, type ClerkEnv } from "../auth.js";

export interface ChatEnv extends ClerkEnv, McpEnv, ChatModelEnv {
  /** Exact Clerk user id of the single permitted chat owner. Required. */
  CHAT_OWNER_USER_ID?: string;
  /**
   * Deliberate operator allowlist for the first verified-user cohort.
   * A comma-separated list of Clerk user ids; empty/absent keeps chat
   * owner-only, while a non-empty malformed value disables chat entirely.
   */
  CHAT_COHORT_USER_IDS?: string;
  /** OpenRouter secret key; model calls fail closed without it. */
  OPENROUTER_API_KEY?: string;
  /** Legacy preference: only curated free IDs can set the initial model. Paid selection requires explicit owner/budget authorization. */
  OPENROUTER_MODEL?: string;
  /** Durable Object namespace bound by index/worker entrypoints. */
  Chat?: DurableObjectNamespace;
  /** Static asset binding for /chat.html and hashed bundles. */
  ASSETS?: Fetcher;
}

/** Safe model identifier: operator-configured chat model or the default. */
export function chatModelIdentifier(env: ChatEnv): string {
  return selectChatModel(defaultChatModelKey(env), env, "").id;
}

export type ClerkSessionCheck =
  | { kind: "active"; expiresAt: number }
  | { kind: "inactive" }
  | { kind: "error" };

/**
 * Live Clerk session check via Backend API sessions.getSession (official
 * SDK; a valid session JWT alone is not sufficient). Returns the verified
 * session expiry (UTC ms) only for the expected user with an active,
 * unexpired session. Clerk transport/outage failures return error and raw
 * Clerk errors are never surfaced.
 */
export async function verifyActiveClerkSession(
  env: ChatEnv,
  sessionId: string,
  expectedUserId: string,
): Promise<ClerkSessionCheck> {
  const secretKey = env.CLERK_SECRET_KEY ?? "";
  const publishableKey = env.CLERK_PUBLISHABLE_KEY ?? "";
  if (!secretKey || !publishableKey) return { kind: "error" };
  try {
    const clerk = createClerkClient({
      secretKey,
      publishableKey,
      jwtKey: env.CLERK_JWT_KEY,
      telemetry: { disabled: true },
    });
    const session = await clerk.sessions.getSession(sessionId);
    // Verified against the installed SDK and live Clerk BAPI: expire_at is UTC milliseconds.
    const expiresAt = session.expireAt;
    const active =
      session.status === "active" &&
      session.userId === expectedUserId &&
      Number.isFinite(expiresAt) &&
      expiresAt > Date.now();
    return active ? { kind: "active", expiresAt } : { kind: "inactive" };
  } catch {
    // Fail closed; never echo raw Clerk errors or config values.
    return { kind: "error" };
  }
}

interface ChatSession {
  userId: string;
  sessionId: string;
  tenant: Tenant;
  /** Verified live Clerk session expiry in UTC milliseconds. */
  expiresAt: number;
}

export type ChatAuthOutcome =
  | { kind: "authenticated"; session: ChatSession; headers: Headers }
  | { kind: "handshake"; headers: Headers }
  | { kind: "unauthenticated" }
  | { kind: "error"; status: number; message: string };

const OWNER_DISABLED_MESSAGE = "Chat is not available.";
const COHORT_DISABLED_MESSAGE = "Chat is not available.";
const SESSION_UNVERIFIED_MESSAGE = "Chat session could not be verified.";

const CHAT_USER_ID_PATTERN = /^user_[A-Za-z0-9]{1,64}$/;

export type ChatCohortConfig =
  | { kind: "valid"; userIds: ReadonlySet<string> }
  | { kind: "malformed" };

/**
 * Parse the deliberate first-cohort allowlist. An absent or empty setting is
 * intentionally owner-only. A non-empty setting must be a duplicate-free list
 * of syntactically valid Clerk user ids; anything else fails closed so an
 * operator typo can never silently broaden or disable part of the cohort.
 */
export function chatCohortUserIds(env: ChatEnv): ChatCohortConfig {
  const raw = env.CHAT_COHORT_USER_IDS;
  if (raw === undefined || raw.trim() === "") return { kind: "valid", userIds: new Set() };
  const entries = raw.split(",").map((value) => value.trim());
  const userIds = new Set<string>();
  for (const entry of entries) {
    if (!entry || !CHAT_USER_ID_PATTERN.test(entry) || userIds.has(entry))
      return { kind: "malformed" };
    userIds.add(entry);
  }
  return { kind: "valid", userIds };
}

/**
 * Authenticate a chat request end to end: verified Clerk identity + tenant
 * bootstrap (existing authenticate()), then exact configured owner or explicit
 * cohort member, active tenant, and a live Clerk session. Missing owner or
 * malformed cohort configuration is a hard 503.
 */
export async function authenticateChatOwner(
  request: Request,
  env: ChatEnv,
  repo?: HostedRepository,
): Promise<ChatAuthOutcome> {
  const owner = (env.CHAT_OWNER_USER_ID ?? "").trim();
  if (!owner) return { kind: "error", status: 503, message: OWNER_DISABLED_MESSAGE };
  const cohort = chatCohortUserIds(env);
  if (cohort.kind === "malformed")
    return { kind: "error", status: 503, message: COHORT_DISABLED_MESSAGE };
  const authOutcome: AuthOutcome = await authenticate(
    request,
    env,
    repo ?? new HostedRepository(env.DB),
  );
  if (authOutcome.kind !== "authenticated") return authOutcome;

  const session = authOutcome.session;
  if (session.tenant.status !== "active") {
    return { kind: "error", status: 503, message: OWNER_DISABLED_MESSAGE };
  }
  if (session.userId !== owner && !cohort.userIds.has(session.userId)) {
    return { kind: "error", status: 403, message: "Not authorized." };
  }
  const check = await verifyActiveClerkSession(env, session.sessionId, session.userId);
  if (check.kind === "error") {
    return { kind: "error", status: 502, message: SESSION_UNVERIFIED_MESSAGE };
  }
  if (check.kind !== "active") return { kind: "unauthenticated" };
  return {
    kind: "authenticated",
    session: {
      userId: session.userId,
      sessionId: session.sessionId,
      tenant: session.tenant,
      expiresAt: check.expiresAt,
    },
    headers: authOutcome.headers,
  };
}

/**
 * Server-derived opaque conversation name: SHA-256 digest of the stable
 * internal tenant id, so the raw tenant UUID never appears in URLs, DO
 * instance names, draft keys, or API responses.
 */
export async function chatConversationDigest(tenantId: string): Promise<string> {
  const digest = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode("gog-hosted-chat:1:" + tenantId).slice(),
    ),
  );
  return Array.from(digest, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/** Opaque draft key per owner/tenant; derived server-side like the conversation. */
export async function chatDraftKey(tenantId: string): Promise<string> {
  return chatConversationDigest(tenantId);
}
