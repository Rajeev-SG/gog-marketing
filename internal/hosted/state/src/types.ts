/**
 * Domain types for the hosted D1 state. These reuse the existing typed
 * concepts from the Go control-plane (tenants, connections, resource grants,
 * audit events) adapted for the Cloudflare Worker / D1 context.
 *
 * Every record carries a tenant_id so that all queries and mutations are
 * tenant-scoped server-side. Ownership is enforced by compound foreign keys
 * in the SQL schema, not only by query filtering.
 */

export type TenantStatus = "active" | "suspended" | "deleted";

export interface Tenant {
  id: string;
  clerkUserId: string;
  status: TenantStatus;
  createdAt: string;
  updatedAt: string;
}

export type ConnectionStatus = "active" | "needs_reconnect" | "revoked";

/**
 * Persisted per-connection resource-discovery outcome. An empty string means
 * no discovery run has been recorded for the current grant state. These are
 * distinct outcomes: `empty` is a successful run with zero resources;
 * `unavailable` means the Go discovery runner could not be reached (#63);
 * `error` means the runner ran but Google/API discovery failed.
 */
export type DiscoveryState = "ok" | "empty" | "unavailable" | "error" | "";

export interface GoogleConnection {
  id: string;
  tenantId: string;
  googleSubject: string;
  email: string;
  displayName: string;
  grantedScopesJson: string;
  status: ConnectionStatus;
  lastError: string;
  lastValidatedAt: string | null;
  discoveryState: DiscoveryState;
  discoveryDetail: string;
  discoveryCheckedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type OAuthIntent = "connect" | "reconnect";

/**
 * One-use OAuth state/PKCE binding created at connection start. The state
 * token itself is never stored raw: callers persist only `stateHash`
 * (SHA-256 hex of the token) and look callbacks up by the same hash.
 */
export interface OAuthStateInput {
  stateHash: string;
  tenantId: string;
  connectionId: string;
  clerkUserId: string;
  clerkSessionId: string;
  intent: OAuthIntent;
  codeVerifier: string;
  nonce: string;
  redirectUri: string;
  scopes: string[];
  services: string[];
  expiresAt: string;
}

export interface OAuthStateRecord {
  stateHash: string;
  tenantId: string;
  connectionId: string;
  clerkUserId: string;
  clerkSessionId: string;
  intent: OAuthIntent;
  codeVerifier: string;
  nonce: string;
  redirectUri: string;
  scopes: string[];
  services: string[];
  createdAt: string;
  expiresAt: string;
  consumedAt: string | null;
}

export type OAuthStateTake =
  | { kind: "taken"; state: OAuthStateRecord }
  | { kind: "unknown" }
  | { kind: "replayed" }
  | { kind: "expired" }
  | { kind: "wrong_tenant" }
  | { kind: "wrong_session" };

export interface ConnectionCredentialInput {
  tenantId: string;
  connectionId: string;
  ciphertext: Uint8Array;
  nonce: Uint8Array;
  keyVersion: number;
}

export interface ConnectionCredential {
  tenantId: string;
  connectionId: string;
  ciphertext: Uint8Array;
  nonce: Uint8Array;
  keyVersion: number;
  createdAt: string;
  updatedAt: string;
}

export interface ResourceGrant {
  id: string;
  tenantId: string;
  connectionId: string;
  service: string;
  resourceId: string;
  resourceType: string;
  displayName: string;
  enabled: boolean;
  metadataJson: string;
  discoveredAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type AuditResult = "allow" | "deny" | "error" | "info";

export interface AuditEvent {
  id: string;
  tenantId: string;
  connectionId: string;
  actorClerkUserId: string;
  action: string;
  result: AuditResult;
  detailJson: string;
  latencyMs: number | null;
  createdAt: string;
}

export interface AuditEventInput {
  id: string;
  tenantId: string;
  connectionId: string;
  actorClerkUserId: string;
  action: string;
  result: AuditResult;
  detailJson: string;
  latencyMs?: number | null;
}

export interface QuotaCounter {
  tenantId: string;
  period: string;
  counter: string;
  value: number;
  updatedAt: string;
}

/**
 * D1-compatible database interface. Matches the Cloudflare D1Database subset
 * the repository uses: `prepare` returns a prepared statement object and
 * `batch` takes an array of those bound prepared statements, the same shape
 * native D1 uses. This keeps binding parameters attached to their statements
 * for both adapters.
 */
export interface D1CompatibleStatement {
  bind(...values: unknown[]): D1CompatibleStatement;
  all<T>(): Promise<{ results?: T[] }>;
  first<T>(): Promise<T | null>;
  run(): Promise<{ success: boolean; meta?: unknown }>;
}

export interface D1CompatibleResult<T = unknown> {
  success: boolean;
  results?: T[];
  meta?: unknown;
}

export interface Database {
  prepare(sql: string): D1CompatibleStatement;
  batch<T = unknown>(statements: D1CompatibleStatement[]): Promise<D1CompatibleResult<T>[]>;
}
