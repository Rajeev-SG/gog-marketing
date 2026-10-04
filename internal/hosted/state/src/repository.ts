import { toByteArray } from "./bytes.js";
import { assertSafeAuditString, sanitizeAuditDetail } from "./audit-detail.js";
import type {
  AuditEvent,
  AuditEventInput,
  ConnectionCredentialInput,
  ConnectionStatus,
  Database,
  GoogleConnection,
  QuotaCounter,
  ResourceGrant,
  Tenant,
} from "./types.js";

/**
 * Typed repository for the hosted D1 state.
 *
 * Every method takes an explicit tenantId parameter so that all queries and
 * mutations are tenant-scoped server-side. The only exception is
 * `bootstrapTenant`, which is the single server-side Clerk→tenant lookup
 * used during authentication before a tenant is known.
 */
export class HostedRepository {
  readonly db: Database;

  constructor(db: Database) {
    this.db = db;
  }

  // ─── Tenant lookup (server-only bootstrap exception) ──────────────────

  /**
   * Resolve or create the tenant for a Clerk user. This is the ONLY method
   * that does not take a tenantId, because it is the bootstrap path that
   * maps a Clerk identity to a stable internal tenant. It is idempotent:
   * calling it twice with the same clerkUserId returns the same tenant.
   */
  async bootstrapTenant(clerkUserId: string): Promise<Tenant> {
    const now = new Date().toISOString();
    const id = crypto.randomUUID();

    // Single racing-safe statement: at most one caller inserts; every other
    // concurrent caller falls through to the authoritative reselect below.
    await this.db
      .prepare(
        `INSERT INTO hosted_tenants (id, clerk_user_id, status, created_at, updated_at)
         VALUES (?1, ?2, 'active', ?3, ?4)
         ON CONFLICT (clerk_user_id) DO NOTHING`,
      )
      .bind(id, clerkUserId, now, now)
      .run();

    // Always reselect so concurrent callers receive the same stable tenant,
    // including the caller whose INSERT was a no-op.
    const row = await this.db
      .prepare("SELECT * FROM hosted_tenants WHERE clerk_user_id = ?1")
      .bind(clerkUserId)
      .first<Record<string, unknown>>();
    if (!row) throw new Error("bootstrapTenant: tenant read-back returned null");
    return mapTenant(row);
  }

  async getTenant(tenantId: string): Promise<Tenant | null> {
    const row = await this.db
      .prepare("SELECT * FROM hosted_tenants WHERE id = ?1")
      .bind(tenantId)
      .first<Record<string, unknown>>();
    return row ? mapTenant(row) : null;
  }

  async getTenantByClerkId(clerkUserId: string): Promise<Tenant | null> {
    const row = await this.db
      .prepare("SELECT * FROM hosted_tenants WHERE clerk_user_id = ?1")
      .bind(clerkUserId)
      .first<Record<string, unknown>>();
    return row ? mapTenant(row) : null;
  }

  // ─── Google connections (tenant-scoped) ─────────────────────────────────

  async createConnection(
    tenantId: string,
    googleSubject: string,
    email: string,
    displayName: string,
    grantedScopesJson: string,
  ): Promise<GoogleConnection> {
    const now = new Date().toISOString();
    const id = crypto.randomUUID();

    await this.db
      .prepare(
        `INSERT INTO hosted_google_connections
           (id, tenant_id, google_subject, email, display_name, granted_scopes_json,
            status, last_error, last_validated_at, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, 'active', '', NULL, ?7, ?8)`,
      )
      .bind(id, tenantId, googleSubject, email, displayName, grantedScopesJson, now, now)
      .run();

    return {
      id,
      tenantId,
      googleSubject,
      email,
      displayName,
      grantedScopesJson,
      status: "active",
      lastError: "",
      lastValidatedAt: null,
      createdAt: now,
      updatedAt: now,
    };
  }

  async listConnections(tenantId: string): Promise<GoogleConnection[]> {
    const result = await this.db
      .prepare("SELECT * FROM hosted_google_connections WHERE tenant_id = ?1 ORDER BY created_at")
      .bind(tenantId)
      .all<Record<string, unknown>>();
    return (result.results ?? []).map(mapConnection);
  }

  async getConnection(tenantId: string, connectionId: string): Promise<GoogleConnection | null> {
    const row = await this.db
      .prepare("SELECT * FROM hosted_google_connections WHERE id = ?1 AND tenant_id = ?2")
      .bind(connectionId, tenantId)
      .first<Record<string, unknown>>();
    return row ? mapConnection(row) : null;
  }

  async updateConnectionStatus(
    tenantId: string,
    connectionId: string,
    status: ConnectionStatus,
    lastError: string,
  ): Promise<void> {
    const now = new Date().toISOString();
    await this.db
      .prepare(
        "UPDATE hosted_google_connections SET status = ?3, last_error = ?4, updated_at = ?5 WHERE id = ?1 AND tenant_id = ?2",
      )
      .bind(connectionId, tenantId, status, lastError, now)
      .run();
  }

  async deleteConnection(tenantId: string, connectionId: string): Promise<void> {
    await this.db
      .prepare("DELETE FROM hosted_google_connections WHERE id = ?1 AND tenant_id = ?2")
      .bind(connectionId, tenantId)
      .run();
  }

  // ─── Connection credentials (ciphertext/nonce/key_version only) ─────────

  async upsertCredential(input: ConnectionCredentialInput): Promise<void> {
    const now = new Date().toISOString();
    await this.db
      .prepare(
        `INSERT INTO hosted_connection_credentials
           (tenant_id, connection_id, ciphertext, nonce, key_version, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
         ON CONFLICT (tenant_id, connection_id)
         DO UPDATE SET ciphertext = ?3, nonce = ?4, key_version = ?5, updated_at = ?7`,
      )
      .bind(
        input.tenantId,
        input.connectionId,
        input.ciphertext,
        input.nonce,
        input.keyVersion,
        now,
        now,
      )
      .run();
  }

  async getCredential(
    tenantId: string,
    connectionId: string,
  ): Promise<{ ciphertext: Uint8Array; nonce: Uint8Array; keyVersion: number } | null> {
    const row = await this.db
      .prepare(
        "SELECT ciphertext, nonce, key_version FROM hosted_connection_credentials WHERE tenant_id = ?1 AND connection_id = ?2",
      )
      .bind(tenantId, connectionId)
      .first<{
        ciphertext: Uint8Array;
        nonce: Uint8Array;
        key_version: number;
      }>();
    if (!row) return null;
    return {
      ciphertext: toByteArray(row.ciphertext),
      nonce: toByteArray(row.nonce),
      keyVersion: row.key_version,
    };
  }

  async deleteCredential(tenantId: string, connectionId: string): Promise<void> {
    await this.db
      .prepare(
        "DELETE FROM hosted_connection_credentials WHERE tenant_id = ?1 AND connection_id = ?2",
      )
      .bind(tenantId, connectionId)
      .run();
  }

  // ─── Resource grants (compound-FK enforced) ────────────────────────────

  async upsertResourceGrant(
    tenantId: string,
    connectionId: string,
    service: string,
    resourceId: string,
    resourceType: string,
    displayName: string,
    enabled: boolean,
    metadataJson: string,
  ): Promise<ResourceGrant> {
    const now = new Date().toISOString();
    const id = crypto.randomUUID();

    await this.db
      .prepare(
        `INSERT INTO hosted_resource_grants
           (id, tenant_id, connection_id, service, resource_id, resource_type,
            display_name, enabled, metadata_json, discovered_at, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
         ON CONFLICT (tenant_id, connection_id, service, resource_id)
         DO UPDATE SET
           resource_type = ?6, display_name = ?7, enabled = ?8,
           metadata_json = ?9, updated_at = ?12`,
      )
      .bind(
        id,
        tenantId,
        connectionId,
        service,
        resourceId,
        resourceType,
        displayName,
        enabled ? 1 : 0,
        metadataJson,
        now,
        now,
        now,
      )
      .run();

    // Read back the actual id (upsert may have used an existing row).
    const row = await this.db
      .prepare(
        "SELECT * FROM hosted_resource_grants WHERE tenant_id = ?1 AND connection_id = ?2 AND service = ?3 AND resource_id = ?4",
      )
      .bind(tenantId, connectionId, service, resourceId)
      .first<Record<string, unknown>>();
    if (!row) throw new Error("upsertResourceGrant: read-back returned null");
    return mapGrant(row);
  }

  async listResourceGrants(tenantId: string, connectionId: string): Promise<ResourceGrant[]> {
    const result = await this.db
      .prepare(
        "SELECT * FROM hosted_resource_grants WHERE tenant_id = ?1 AND connection_id = ?2 ORDER BY service, resource_id",
      )
      .bind(tenantId, connectionId)
      .all<Record<string, unknown>>();
    return (result.results ?? []).map(mapGrant);
  }

  async getEnabledResourceGrant(
    tenantId: string,
    connectionId: string,
    service: string,
    resourceId: string,
  ): Promise<ResourceGrant | null> {
    const row = await this.db
      .prepare(
        "SELECT * FROM hosted_resource_grants WHERE tenant_id = ?1 AND connection_id = ?2 AND service = ?3 AND resource_id = ?4 AND enabled = 1",
      )
      .bind(tenantId, connectionId, service, resourceId)
      .first<Record<string, unknown>>();
    return row ? mapGrant(row) : null;
  }

  async setResourceEnabled(
    tenantId: string,
    connectionId: string,
    service: string,
    resourceId: string,
    enabled: boolean,
  ): Promise<void> {
    const now = new Date().toISOString();
    await this.db
      .prepare(
        `UPDATE hosted_resource_grants SET enabled = ?5, updated_at = ?6
         WHERE tenant_id = ?1 AND connection_id = ?2 AND service = ?3 AND resource_id = ?4`,
      )
      .bind(tenantId, connectionId, service, resourceId, enabled ? 1 : 0, now)
      .run();
  }

  async deleteResourceGrant(
    tenantId: string,
    connectionId: string,
    service: string,
    resourceId: string,
  ): Promise<void> {
    await this.db
      .prepare(
        "DELETE FROM hosted_resource_grants WHERE tenant_id = ?1 AND connection_id = ?2 AND service = ?3 AND resource_id = ?4",
      )
      .bind(tenantId, connectionId, service, resourceId)
      .run();
  }

  // ─── Audit (safe metadata only) ─────────────────────────────────────────

  async appendAudit(input: AuditEventInput): Promise<void> {
    const now = new Date().toISOString();
    // These stored labels must not become alternate raw-credential channels.
    for (const value of [input.id, input.action, input.actorClerkUserId]) {
      assertSafeAuditString(value);
    }
    const resultValues = new Set(["allow", "deny", "error", "info"]);
    if (!resultValues.has(input.result)) {
      throw new Error("appendAudit: unsupported audit result");
    }
    if (typeof input.action !== "string" || input.action.length === 0) {
      throw new Error("appendAudit: audit action must be a non-empty string");
    }

    // The audit table's FK only enforces tenant ownership, so a connectionId
    // is verified against the same tenant before it may be recorded.
    if (input.connectionId) {
      const owned = await this.db
        .prepare(
          "SELECT 1 AS owned FROM hosted_google_connections WHERE id = ?1 AND tenant_id = ?2",
        )
        .bind(input.connectionId, input.tenantId)
        .first<{ owned: number }>();
      if (!owned) throw new Error("appendAudit: connection not owned by tenant");
    }

    // Hosted v1 has exactly one Clerk user per tenant: the recorded actor
    // must be that user, not an identity supplied by another tenant.
    const actorOwned = await this.db
      .prepare("SELECT 1 AS owned FROM hosted_tenants WHERE id = ?1 AND clerk_user_id = ?2")
      .bind(input.tenantId, input.actorClerkUserId)
      .first<{ owned: number }>();
    if (!actorOwned) throw new Error("appendAudit: actor not owned by tenant");

    const detailJson = sanitizeAuditDetail(input.detailJson);

    await this.db
      .prepare(
        `INSERT INTO hosted_audit_events
           (id, tenant_id, connection_id, actor_clerk_user_id, action, result,
            detail_json, latency_ms, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)`,
      )
      .bind(
        input.id,
        input.tenantId,
        input.connectionId,
        input.actorClerkUserId,
        input.action,
        input.result,
        detailJson,
        input.latencyMs ?? null,
        now,
      )
      .run();
  }

  async listAudit(tenantId: string, limit: number): Promise<AuditEvent[]> {
    const result = await this.db
      .prepare(
        "SELECT * FROM hosted_audit_events WHERE tenant_id = ?1 ORDER BY created_at DESC LIMIT ?2",
      )
      .bind(tenantId, limit)
      .all<Record<string, unknown>>();
    return (result.results ?? []).map(mapAudit);
  }

  // ─── Quota counters (atomic increments) ─────────────────────────────────

  /**
   * Atomically increment a quota counter and return the value this caller
   * produced. The UPSERT ... RETURNING form is a single SQL statement, so
   * concurrent callers cannot read each other's write mid-flight: each
   * returns its own serialised running total (1..N under N concurrent calls).
   */
  async incrementQuota(
    tenantId: string,
    period: string,
    counter: string,
    delta: number,
  ): Promise<number> {
    const now = new Date().toISOString();
    if (!Number.isInteger(delta) || delta < 0) {
      throw new Error("incrementQuota: delta must be a non-negative integer");
    }

    const row = await this.db
      .prepare(
        `INSERT INTO hosted_quota_counters (tenant_id, period, counter, value, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5)
         ON CONFLICT (tenant_id, period, counter)
         DO UPDATE SET value = value + excluded.value, updated_at = excluded.updated_at
         RETURNING value`,
      )
      .bind(tenantId, period, counter, delta, now)
      .first<{ value: number }>();
    if (!row) throw new Error("incrementQuota: value read-back returned null");
    return Number(row.value);
  }

  async getQuota(tenantId: string, period: string, counter: string): Promise<QuotaCounter | null> {
    const row = await this.db
      .prepare(
        "SELECT * FROM hosted_quota_counters WHERE tenant_id = ?1 AND period = ?2 AND counter = ?3",
      )
      .bind(tenantId, period, counter)
      .first<Record<string, unknown>>();
    if (!row) return null;
    return mapQuota(row);
  }
}

// ─── Row mappers ────────────────────────────────────────────────────────────

function mapTenant(row: Record<string, unknown>): Tenant {
  return {
    id: String(row.id),
    clerkUserId: String(row.clerk_user_id),
    status: String(row.status) as Tenant["status"],
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapConnection(row: Record<string, unknown>): GoogleConnection {
  return {
    id: String(row.id),
    tenantId: String(row.tenant_id),
    googleSubject: String(row.google_subject),
    email: String(row.email),
    displayName: String(row.display_name),
    grantedScopesJson: String(row.granted_scopes_json),
    status: String(row.status) as GoogleConnection["status"],
    lastError: String(row.last_error),
    lastValidatedAt: row.last_validated_at ? String(row.last_validated_at) : null,
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapGrant(row: Record<string, unknown>): ResourceGrant {
  return {
    id: String(row.id),
    tenantId: String(row.tenant_id),
    connectionId: String(row.connection_id),
    service: String(row.service),
    resourceId: String(row.resource_id),
    resourceType: String(row.resource_type),
    displayName: String(row.display_name),
    enabled: Number(row.enabled) === 1,
    metadataJson: String(row.metadata_json),
    discoveredAt: row.discovered_at ? String(row.discovered_at) : null,
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapAudit(row: Record<string, unknown>): AuditEvent {
  return {
    id: String(row.id),
    tenantId: String(row.tenant_id),
    connectionId: String(row.connection_id),
    actorClerkUserId: String(row.actor_clerk_user_id),
    action: String(row.action),
    result: String(row.result) as AuditEvent["result"],
    detailJson: String(row.detail_json),
    latencyMs: row.latency_ms != null ? Number(row.latency_ms) : null,
    createdAt: String(row.created_at),
  };
}

function mapQuota(row: Record<string, unknown>): QuotaCounter {
  return {
    tenantId: String(row.tenant_id),
    period: String(row.period),
    counter: String(row.counter),
    value: Number(row.value),
    updatedAt: String(row.updated_at),
  };
}
