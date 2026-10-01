# Control Plane

`gog controlplane` is the first hosted SaaS control-plane slice. It keeps the
existing Google integration engine, but moves connection identity, OAuth state,
resource grants, and audit events into a relational control-plane store.

## Run locally

The command accepts an explicit `memory://` store for a disposable local run.
Use Postgres for anything that must survive restarts:

```bash
gog controlplane \
  --listen 127.0.0.1:8080 \
  --database-url postgres://user:password@localhost:5432/gog_control_plane \
  --secret-backend secret-manager \
  --secret-manager-project my-gcp-project \
  --owner-email owner@example.com \
  --admin-token "$GOG_CONTROL_PLANE_ADMIN_TOKEN" \
  --google-client-name personal-owned \
  --session-key "$GOG_CONTROL_PLANE_SESSION_KEY"
```

The secret backend is an explicit required choice. For a hermetic loopback smoke run, use `--database-url memory://`, `--secret-backend file`, and a loopback `--listen` address; the file backend encrypts values with the supplied `--master-key` and stores only ciphertext. The server refuses `file` on a non-loopback listener. `--admin-token` is required and is checked in constant time before a web session is issued.

`--google-client-name` loads the public client ID and protected client secret through the existing `gog` credentials store. On macOS, use a stable Developer ID-signed installed binary, such as `~/.codex/scripts/gog-stable-auth.sh` on Rajeev's local rig. Do not use `go run` or a rebuilt ad-hoc `bin/gog`; macOS treats every rebuild as a new Keychain application. For development and automation, prefer the encrypted file keyring or explicit `--google-client-id` and `--google-client-secret`. Do not combine the named and explicit client forms.

The OAuth redirect URI is derived from `--external-base-url` and is
`/oauth/google/callback`. Register that exact URI on the central gog-marketing
OAuth client. Customers do not provide OAuth client JSON.

## Data and secrets

Postgres migrations create users, organisations, memberships, Google
connections, resource grants, audit events, and single-use OAuth states. A
connection row stores a `token_secret_ref`, never refresh/access token
plaintext.

- `file`: local/test encrypted secret store.
- `secret-manager`: Google Secret Manager versions, using managed encryption
  at rest. The database stores only the version resource name.

## Product flow

The normal `/` surface is the marketer product shell:

1. Sign in with Google.
2. Choose **Connect Google**.
3. Complete the existing connection OAuth flow.
4. Discovery runs automatically, then the product shows grouped assets.
5. Select assets and save.

The normal product surface does not ask for Google Cloud Console, OAuth client
JSON, scopes, service identifiers, GCP projects, tokens, or CLI commands. The v1
product shell is single-user and accepts only the configured workspace owner;
multi-member invitations are outside this slice.

## Operator/admin flow

The low-level control-plane tools remain at `/admin` for operators and support:

1. Sign in as the configured owner/admin.
2. Open **Connections**. The admin path retains named connection and diagnostics controls.
3. Select a connection and choose **Connect Google**.
4. Complete the central Google OAuth consent flow.
5. Run **Refresh and discover**.
6. Enable only the Workspace or Marketing services the customer needs. For
   resource-backed marketing services, choose the specific GA4, GTM, Google
   Ads, Search Console, or BigQuery resources; for capabilities without a
   stable resource picker, enable the service/tool grant.
7. Reconnect or disconnect from the same detail page.

The policy check is intentionally reusable outside the UI:

```text
Google permits resource
        AND
resource is enabled in gog-marketing
        ↓
request may proceed
```

`controlplane.Policy.Allow` gates resource reads and
`controlplane.Policy.AllowTool` gates curated service/tool reads. Unknown
resources, unknown tools, wildcard grants, disabled grants, and
cross-organisation connection IDs are denied by default.

Discovery is atomic across configured services. Google Ads is skipped only when
its developer token is absent; once configured, an API or permission failure
fails the discovery operation rather than silently returning partial results.

## Product resource reads

The product exposes `GET /api/connections/{id}/resource?resource=<resource-id>`
for permission-controlled reads through the agent/API path. The endpoint
requires `Sec-Fetch-Site: same-origin|none` and a matching `X-CSRF-Token`
header (browser-only; non-browser clients are intentionally unsupported).

Reads are gated by `Policy.Allow` before any Google API call. Disabled or
cross-account resources return `403 access_denied` without calling Google.
Available resources return a fixed-schema JSON response with the resource ID,
type, and display name.

The analytics property endpoint `GET /api/connections/{id}/analytics/property`
uses the same gate and returns a fixed property schema.

## Product service/tool reads

Capabilities without a stable resource picker use explicit service/tool grants
instead of synthetic resource IDs. The current curated read-only selectors are
`gmail_search`, `calendar_events`, and `drive_search`; other services remain
excluded from the hosted tool path until a safe per-tool selector is added. The product exposes
`GET /api/connections/{id}/tool?service=<service>&tool=<tool>` for representative
read-only tools. It requires the same browser-only Fetch Metadata and CSRF
checks as resource reads, runs `Policy.AllowTool` before token retrieval, and
uses the stored central token through the existing typed Google clients. Tool
results are fixed-schema JSON and every allow/deny/error path is audited. No
wildcard tool grant is accepted.

## Acceptance evidence policy

Do not commit screenshots or manifests containing real account emails, Google
resource IDs, tokens, auth codes, or customer names. Store those artifacts in a
private operator-controlled evidence location. The public repository may contain
only a redacted procedure and synthetic acceptance examples.


## Security boundaries

- Control-plane sign-in requires the configured bootstrap admin token and Google service consent remains separate.
- Session cookies are HttpOnly, SameSite=Strict, signed, and expire.
- PKCE S256 and OIDC nonce are generated per flow, stored with the single-use state, and verified during exchange.
- Mutating routes require a per-session CSRF token.
- OAuth state is single-use, expires after ten minutes, and is bound to the
  connection and organisation.
- Refresh/access tokens are never rendered in the UI or included in audit
  details.
- Disconnect deletes the stored credential and attempts Google revocation.

## Verification

Offline tests cover persistence relationships, migration/rollback contracts,
OAuth success/denial/invalid-state/reconnect behavior, secret-store scoping and
encryption, discovery normalisation, resource policy, authenticated web routes,
and token-free responses.

Live acceptance still requires the two real Google identities and a deployed
OAuth client. Use the UI to confirm `gmail` and `singulyr`, run discovery, make
one read request through the existing engine for each connection, and capture a
redacted acceptance manifest. Do not commit tokens, auth codes, or screenshots
containing them.
