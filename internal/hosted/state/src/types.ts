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
  createdAt: string;
  updatedAt: string;
}

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
