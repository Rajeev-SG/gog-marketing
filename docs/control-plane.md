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

`--google-client-name` loads the public client ID and protected client secret through the existing `gog` credentials store. Use the stable `bin/gog` binary for this path so macOS can keep one persistent Keychain grant instead of prompting for temporary `go run` binaries. Deployments that inject secrets directly may instead provide both `--google-client-id` and `--google-client-secret`; do not combine the named and explicit forms.

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

## Control-plane flow

1. Sign in as the configured owner/admin.
2. Open **Connections**. The first run seeds `gmail` and `singulyr`.
3. Select a connection and choose **Connect Google**.
4. Complete the central Google OAuth consent flow.
5. Run **Refresh and discover**.
6. Enable only the GA4, GTM, Google Ads, Search Console, or BigQuery resources
   the customer wants gog-marketing to expose.
7. Reconnect or disconnect from the same detail page.

The policy check is intentionally reusable outside the UI:

```text
Google permits resource
        AND
resource is enabled in gog-marketing
        ↓
request may proceed
```

`controlplane.Policy.Allow` is the future MCP/API/agent gate. Unknown resources
and cross-organisation connection IDs are denied by default.

Discovery is atomic across configured services. Google Ads is skipped only when
its developer token is absent; once configured, an API or permission failure
fails the discovery operation rather than silently returning partial results.

## Acceptance evidence policy

Do not commit screenshots or manifests containing real account emails, Google
resource IDs, tokens, auth codes, or customer names. Store those artifacts in a
private operator-controlled evidence location. The public repository may contain
only a redacted procedure and synthetic acceptance examples.


## Security boundaries

- Control-plane sign-in requires the configured bootstrap admin token and Google marketing consent remains separate.
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
