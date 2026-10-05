# Hosted Google account connections (#62)

The hosted Google OAuth connection lifecycle is separate from Clerk identity:
Clerk answers *who is signed in*; Google OAuth answers *which Google account
and which scopes gog-marketing may use*. The lifecycle lives in the existing
`gog-marketing` Worker (`internal/hosted/app/`) and persists state through the
existing D1 database (`internal/hosted/state/`).

## Flow

1. **Start** — `POST /api/google/connect` (Clerk-authenticated). The Worker
   creates a pending connection placeholder, generates a one-use OAuth state
   binding (server-side state hash, PKCE verifier, OIDC nonce) tied to the
   verified tenant, the exact Clerk session ID (`sid`), and the intended
   connection, then returns Google's authorization URL. Scopes are
   incremental and least-privilege: identity scopes (`openid email
   userinfo.email`) plus read-only scopes for the explicitly selected hosted
   services (analytics, googleads, tagmanager, searchconsole, bigquery,
   gmail, calendar, drive). No write scopes are offered. Gmail and Drive
   read-only scopes are restricted; they are available only to the deliberate
   two-user Testing owner cohort. Public launch remains gated on scope review
   and verification under #69.
2. **Consent** — the user completes Google's consent screen.
3. **Callback** — `GET /oauth/google/callback`. The canonical registered
   redirect URI is
   `https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback`.
   The Worker re-authenticates the Clerk session, atomically consumes the
   one-use state record, verifies tenant/session/connection binding, exchanges
   the code with the stored PKCE verifier, verifies the returned Google
   identity (userinfo subject + verified email; OIDC `nonce`/`aud`
   defense-in-depth), and only then persists AES-GCM encrypted credentials and
   the verified identity.
4. **Discovery** — resource discovery runs through the #63 Go execution
   runner. Until that runner is configured, the outcome is explicitly
   `unavailable` (`runner_not_configured`): existing grants are never erased or
   expanded, and no inventory is fabricated. Discovered resources persist as
   grants defaulting to **disabled**; existing enable/disable choices are
   preserved on rediscovery.

## Binding and replay resistance

- The state token is 256-bit random; only its SHA-256 hash is stored.
- Callbacks must present the raw state, and the stored record must match the
  authenticated tenant, the exact Clerk session ID that started the flow, and
  an existing connection (compound FK cascades the record with its
  connection).
- Consumption is atomic (`UPDATE ... consumed_at IS NULL RETURNING`), so a
  second callback for the same state is rejected as a replay. Cross-tenant and
  sibling-session callbacks are rejected without consuming the binding.
- The same Google identity (subject or verified email) cannot be connected to
  two connections; reconnects require immutable subject equality on that
  connection. A conflicting callback never replaces another connection.

## Credentials at rest

Refresh credentials are stored as AES-256-GCM ciphertext + 12-byte nonce +
`key_version` only (`hosted_connection_credentials`). The root key is a
Worker secret, never D1/KV/git:

| Variable | Meaning |
| --- | --- |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY` | Standard base64 of 32 random bytes; provisioned as key_version `1`. |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS` | Optional rotation map `{"<version>": "<base64 32B>"}`; encryption always uses the highest version, decryption resolves the stored version. |

Each ciphertext is authenticated against the tenant, connection, and key
version (AES-GCM AAD), so moving a ciphertext between connections fails.
Credential material never appears in API responses, logs, MCP payloads, or
audit rows; audit details stay inside the state package's allowlist.

## Environment

Set on the Worker by the coordinator (values never committed):

| Variable | Kind | Purpose |
| --- | --- | --- |
| `GOG_GOOGLE_OAUTH_CLIENT_ID` | non-secret | Central web OAuth client ID (project `gog-marketing-prod`). |
| `GOG_GOOGLE_OAUTH_CLIENT_SECRET` | secret | Central web OAuth client secret. |
| `GOG_GOOGLE_OAUTH_REDIRECT_URI` | non-secret | `https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback` (must match the registered URI exactly). |
| `GOG_GOOGLE_OAUTH_AUTH_URL` / `_TOKEN_URL` / `_USERINFO_URL` / `_REVOKE_URL` | non-secret overrides | Staging/test endpoint overrides; defaults are Google's canonical endpoints. |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY` | secret | Root encryption key (32-byte base64, version 1). |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS` | secret (optional) | Rotation map (see above). |
| `GOG_CLOUD_RUN_SERVICE_URL` | non-secret | #63 Go discovery runner endpoint (absent until #63). |
| `GOG_RUNNER_INVOCATION_TOKEN` | secret | Runner authentication token. |
| `GOG_HOSTED_OAUTH_STATE_TTL_SECONDS` | non-secret (optional) | State lifetime; default 600 s. |

## Routes

| Route | Auth | Behaviour |
| --- | --- | --- |
| `GET /api/google/connections` | required | Tenant-scoped connection list: verified identity, status, scopes, discovery outcome. No credentials, no tenant ID. |
| `POST /api/google/connect` | required | Starts the flow. Body: `{"services": [...]}` (new connection) or `{"connectionId": "...", "services": [...]}` (reconnect). Returns the bound authorization URL. |
| `GET /oauth/google/callback` | required | Completes the flow; HTML browsers redirect home with a fixed safe outcome, API clients receive the connection view + discovery outcome. |
| `POST /api/google/connections/:id/disconnect` | required | Best-effort Google revocation, then deletes the connection, credentials, and grants. |
| `POST /api/google/connections/:id/refresh` | required | Validates/refreshes the stored credential. |

## Distinct user-facing outcomes

Every error carries both a human message and a machine code; provider
exception text is never echoed:

| Code | HTTP | Meaning |
| --- | --- | --- |
| `operator_config_missing` | 503 | Operator-side Google/encryption configuration missing (never "please reconnect"). |
| `consent_denied` | 403 | The user denied Google consent. |
| `state_unknown` / `state_expired` / `state_replayed` | 400/410/409 | Callback not bound, stale, or already used. |
| `callback_wrong_tenant` / `callback_wrong_session` / `callback_wrong_connection` | 403 | Callback does not match the starting tenant, Clerk session, or intended connection. |
| `google_api_error` / `google_unavailable` | 502 | Google service failure / unreachable. |
| `google_identity_failed` | 403 | Returned identity (or OIDC nonce) could not be verified. |
| `identity_conflict` | 409 | The Google account is already connected elsewhere (or a reconnect would change the account). |
| `connection_not_found` | 404 | The connection no longer exists. |
| `needs_reconnect` | 409 | Google grant expired or was revoked (credential dropped; grants preserved). |
| `discovery_unavailable` / `discovery_error` | — (informational) | Discovery runner not configured (#63) or failed; existing grants untouched. |
| `discovery.empty` (`status: "empty"`, detail `no_resources`) | — (informational) | Discovery succeeded with zero resources — distinct from failure. |

Clerk sign-in failures keep their #61 outcomes (503 not configured, 502
authentication service unavailable, 401 unauthenticated).

## Limitations / follow-ups

- Resource discovery is explicitly unavailable until the #63 Cloud Run Go
  execution service exists; the Worker never re-implements Google discovery.
- OIDC id_token signatures are not verified; the TLS-exchanged access token's
  userinfo call is the identity authority (matches the Go control-plane's
  provider). Nonce and audience claims are checked as defense-in-depth.
- Google revocation on disconnect is best-effort; local credentials are always
  removed, and connection records cascade-delete credentials, grants, and
  OAuth state.
- Real-browser owner acceptance (consent, multi-account, restart persistence)
  is the coordinator's #62 acceptance run against the deployed Worker; the
  unit/integration suites here use real Clerk SDK verification and real
  Miniflare D1 but no live Google consent.

### Repair security contracts

Cookie-bearing POST mutations require an exact same-origin Origin that is in
the configured Clerk authorized-party allowlist, plus application/json. Missing
or foreign Origin, simple forms, and malformed connect JSON fail before any
mutation/provider call. Bearer-only API requests may omit Origin; a supplied
Origin must match. Browser callbacks (Accept: text/html) redirect to the
protected home with only a fixed outcome code; API callbacks retain safe JSON.
When Clerk itself fails terminally (missing configuration 503, provider auth
service failure 502, rejected/expired session 401, inactive tenant 403, or an
unexpected 500), browser callbacks receive a static branded HTML notice — no
Clerk SDK load, no raw auth/provider errors, and the code/state query is
stripped client-side from the address bar — while API callbacks retain the
corresponding safe JSON status.
Reconnect uses immutable Google sub equality, not email, and retains previous
hosted service scopes. Failed new consent/code exchange does not prove the old
stored grant revoked: operator/transient/denial failures leave its health and
credentials unchanged. Only invalid_grant from stored-credential refresh marks
needs_reconnect. Gmail gmail.readonly and Drive drive.readonly are RESTRICTED;
read-only is not a Google verification classification. The current central app
is Testing with two owner test users, not a verified public beta; #69 remains
the public-launch gate.
