# Hosted tenants (multi-account SaaS MVP)

The hosted SaaS layer runs the same gog engine for many accounts, one
isolated, tenant-scoped environment per customer. The CLI remains the
reference implementation: every hosted call is a gog subprocess with a
tenant-specific environment.

## Isolation model

- Each tenant gets its own gog home (`<config>/tenants/<name>`): separate
  config, keyring-backed OAuth tokens, cache, and state. Nothing is shared.
- Hosted calls pin one account and one OAuth client per tenant; a tenant can
  never select another account.
- With a master secret (`GOG_TENANTS_MASTER_KEY` or `--master-key-file`), each
  tenant's file keyring uses an HKDF-derived password, so OAuth tokens are
  encrypted at rest per tenant. Loss of one tenant's password does not unlock
  other tenants.
- Every hosted decision is written to `<tenant-home>/audit/audit.jsonl`:
  timestamp, tool, decision (execute/deny), exit code, and detail. The audit
  trail lives inside the tenant home, so tenants cannot read each other's.

## Registry and lifecycle

```bash
gog tenant add personal rajeev.sgill@gmail.com --readonly
gog tenant add singulyr rajeev@singulyr.com \
  --allow-tools gmail_search,bigquery_query_dry_run \
  --notes singulyr-marketing

gog tenant list
gog tenant get personal
gog tenant audit personal --max 100
gog tenant remove personal          # registry entry + isolated home
gog tenant remove personal --keep-home
```

Use `--oauth-client` when pinning the tenant's stored OAuth client; the global
`--client` flag continues to select the operator runtime client. `--readonly`
is also global and records the tenant as read-only at creation.

The registry (`tenants.json`) holds tenant metadata only. Removing a tenant
deletes its isolated home by default; `--keep-home` preserves data for
forensics or 30-day-style holds.

## Hosted API

```bash
gog tenant serve --port 8086 --master-key-file ~/.gog-tenants-master
```

The server prints the hosted API bearer token once at startup (or pin one
with --serve-token-file); every request needs Authorization: Bearer token and
tokens are compared in constant time. Requests with a browser Origin header or
an unexpected Host are rejected (DNS-rebinding and CSRF hardening). Serving
requires a master secret: tenant OAuth tokens are always encrypted at rest.
Request bodies are capped at 1 MiB, requests time out after 3 minutes, and
concurrent calls are bounded.

Endpoints (bound to 127.0.0.1 only):

- `GET /healthz`
- `GET /tenants` — tenant list (no secrets)
- `POST /tenants/<name>/tools` — tools this tenant may call
- `POST /tenants/<name>/call` — body `{"tool": "...", "arguments": {...}}`

Child environment: hosted children get a minimal environment (PATH, HOME, TERM,
locale, TMPDIR, plus the tenant's GOG_HOME, GOG_ACCOUNT, and file-keyring
settings). Operator secrets and unrelated GOG_* overrides are not inherited.
The per-tenant keyring password is written to a 0600 file inside the tenant
home and passed by file path, so the secret never appears in child-process env.

Policy:

- An explicit `--allow-tools` list is the tenant's exact tool surface.
- Without a list, only read-risk tools are exposed.
- `--readonly` tenants run every child with `--readonly`, which also blocks
  writes at the transport layer.

## Not yet covered (MVP boundary)

- No MCP-over-HTTP transport; the hosted API is a fixed JSON surface over the
  same typed tool builders. Streamable-HTTP MCP hosting is follow-up.
- No per-tenant OS users; isolation is at the gog-home/filesystem/audit layer
  plus process env.
- Master key rotation is manual (re-derive per tenant with the same HKDF info
  once tokens are re-authenticated).
- No hosted billing/quota metering yet; the audit log is the usage record.
