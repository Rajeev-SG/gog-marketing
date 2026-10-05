# Hosted D1 state

The `internal/hosted/state/` package defines the authoritative D1 schema, migrations, and typed repository for hosted v1 product state. D1 is the source of truth for tenants, Google connections, encrypted credentials, resource grants, audit events, and quota counters. KV may hold safe cache or ephemeral data only.

## Schema

Tables (all prefixed `hosted_` to avoid collision with the tracking worker):

| Table | Purpose | Ownership enforcement |
| --- | --- | --- |
| `hosted_tenants` | One Clerk user → one stable tenant | `clerk_user_id UNIQUE` |
| `hosted_google_connections` | Multiple Google accounts per tenant | `UNIQUE (tenant_id, google_subject)` |
| `hosted_connection_credentials` | Ciphertext/nonce/key_version only | Compound FK `(tenant_id, connection_id)` |
| `hosted_resource_grants` | Per-connection service/resource grants | Compound FK `(tenant_id, connection_id)` |
| `hosted_audit_events` | Fixed safe metadata; no codes/tokens/secrets | FK `tenant_id` |
| `hosted_quota_counters` | Atomic quota/rate counters | FK `tenant_id`, PK `(tenant_id, period, counter)` |

### Tenant scoping

Every child table carries `tenant_id`. Queries in the typed repository always scope by `tenant_id`. Cross-tenant writes are rejected by compound foreign keys at the SQL layer, not just by query filtering.

The only method that does not take a `tenantId` is `bootstrapTenant(clerkUserId)` — the single server-side Clerk→tenant lookup during authentication before a tenant is known. It is idempotent.

### Credentials

The `hosted_connection_credentials` table stores only:
- `ciphertext` (BLOB)
- `nonce` (BLOB, IV)
- `key_version` (INTEGER)

No plaintext token, refresh token, or secret column exists. Key rotation is supported by the `key_version` column; different connections can use different key versions simultaneously.

### Audit safety

The `hosted_audit_events` table stores fixed safe metadata only. It has no columns for OAuth codes, tokens, secrets, or raw payloads. `detail_json` is enforced at the storage boundary (`src/audit-detail.ts`): it must be a flat JSON object whose keys are limited to the safe allowlist (`resource`, `resourceType`, `service`, `operation`, `error`, `count`, `page`, `attempt`, `enabled`) with matching string/number/boolean types, finite numbers, and 256-character string caps. The `error` field accepts only stable internal codes: `none`, `permission_denied`, `unauthenticated`, `invalid_request`, `not_found`, `rate_limited`, `quota_exceeded`, `needs_reconnect`, `provider_unavailable`, or `internal_error`; never pass raw provider exceptions. Recognisable credential assignments, authorization headers/schemes, and Google token syntax are rejected in all detail strings and stored audit id/action/actor labels; ordinary identifiers containing “token” are allowed. Unknown keys, nested material, arrays, invalid types, and oversized strings are rejected before the write, and the rejection messages never echo the supplied values. Audit rows may only reference a connection that belongs to the same tenant (`connectionId` is verified against `hosted_google_connections` before insert).

### Quota counters

`hosted_quota_counters` increments with a single `INSERT ... ON CONFLICT DO UPDATE ... RETURNING value` statement, so concurrent callers each receive their own serialised running value (exact `1..N` for `N` concurrent calls) instead of racing a separate `SELECT`. Negative and fractional increments are rejected before reaching SQL.

## Migrations

### Apply locally (dev)

```sh
cd internal/hosted/state
pnpm install --frozen-lockfile
npx wrangler d1 migrations apply gog-marketing --local --config wrangler.migrations.toml
```

The migration binding identifies the existing coordinator-verified D1 resource. Wrangler matches local migrations by `database_name`.

### Apply remotely (deploy)

The coordinator applies migrations against the existing `gog-marketing` D1 database after review. No provider resources are created or mutated by CI.

```sh
npx wrangler d1 migrations apply gog-marketing --remote --config wrangler.migrations.toml
```

Before applying, the coordinator must verify the configured `database_id` matches the intended existing D1 database. Do not commit the real ID to git if it is sensitive in your deployment model.

## Repository

`src/repository.ts` provides a typed `HostedRepository` class that works against the D1-compatible `Database` interface. `prepare` returns a bound-statement object and `batch` takes an array of those bound statements — the same shape native D1 uses — so binding parameters are always preserved. Tenant bootstrap is a single `INSERT ... ON CONFLICT (clerk_user_id) DO NOTHING` statement followed by an authoritative reselect, so concurrent Clerk users always resolve to one stable tenant. Credential BLOB reads are normalised to byte arrays (`src/bytes.ts`) because native D1 returns `number[]` while `node:sqlite` returns `Uint8Array`.

The production entrypoint (`src/index.ts`) exports only the repository and types; it bundles cleanly for Workers with no Node built-ins. `src/dev-migrations.ts` (Node migration loading) and `src/sqlite-adapter.ts` (`node:sqlite` test adapter) are deliberately outside the production import graph.

## Testing

```sh
cd internal/hosted/state
pnpm install --frozen-lockfile
pnpm test          # vitest run (all tests)
pnpm typecheck     # tsc --noEmit
pnpm lint          # oxfmt --check + oxlint --deny-warnings
```

Test coverage:

- **Migration**: applies from empty; idempotent reapply; deterministic schema; lookup indexes exist (Node SQLite and native Miniflare D1).
- **Tenant lookup**: idempotent Clerk→tenant bootstrap; concurrent bootstrap callers resolve to one stable tenant; different users get different tenants.
- **Isolation**: compound FK rejects cross-tenant connection/grant/credential writes; queries return only own data; cascade delete works.
- **Credentials**: schema has no plaintext token columns; ciphertext/nonce/key_version round-trip; native D1 `number[]` BLOB reads are normalised to byte arrays and round-trip through workerd; cross-tenant credential read returns null; key rotation; cascade delete; audit detail does not contain raw credential material.
- **Audit**: strict allowlisted/typed detail metadata at the storage boundary; unsafe writes (tokens, unknown keys, nested material, invalid types, oversized strings) are rejected without storage or value echo; foreign connection associations are rejected.
- **Quota**: single-statement UPSERT ... RETURNING increments (exact `1..N` under concurrency, in Node SQLite and native D1); negative/fractional increments rejected; per-period separation; cross-tenant separation; FK rejection; high-frequency increments.
- **Worker compatibility**: `src/index.ts` bundles for Workers with no Node built-ins; the bundled repository runs inside workerd against a real D1 binding (tenant bootstrap, BLOB roundtrip, concurrent quota).
