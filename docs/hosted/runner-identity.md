# Hosted runner identity

## Purpose

The Worker calls only the private Cloud Run endpoint defined by
`GOG_CLOUD_RUN_SERVICE_URL`, and that URL must be the canonical HTTPS
`/v1/execute` path. There is no public read, grant, or MCP endpoint and no
static bearer fallback.

## Two authentication layers

1. **Native Cloud Run IAM identity.** The Worker signs a short-lived RS256
   WIF caller assertion with the dedicated Worker RSA2048 key, exchanges it at
   the fixed Google STS token URL, and then asks Google IAM Credentials to
   mint an ID token for the configured runner service account. The native ID
   token is sent in `X-Serverless-Authorization`. Native IAM remains enabled;
   this is not Google service-account access-token impersonation.
2. **Application capability.** The Worker signs a separate HS256 JWT with
   `GOG_RUNNER_INVOCATION_TOKEN` and sends it in `Authorization`. The Go
   runner requires the configured worker issuer/subject/audience, integer
   time claims within 60 seconds, the exact tenant/connection/operation, and
   the lowercase SHA-256 digest of the exact UTF-8 JSON body.

## Environment contract

All values below are required together before the private runner is enabled.
Partial or invalid configuration leaves discovery explicitly unavailable and
does not affect the existing Google OAuth connect path:

- `GOG_CLOUD_RUN_SERVICE_URL` — full private HTTPS runner URL ending at
  `/v1/execute`.
- `GOG_RUNNER_WIF_PROVIDER` — the full WIF provider resource path.
- `GOG_RUNNER_WIF_ISSUER` — trusted HTTPS public issuer; it is used only as
  the JWT `iss`, never inferred from the request host.
- `GOG_RUNNER_WIF_KEY_ID` — public key identifier for the RS256 assertion.
- `GOG_RUNNER_WIF_SIGNING_KEY` — operator secret, standard base64 PKCS8 DER.
- `GOG_RUNNER_SERVICE_ACCOUNT` — runner Google service account.
- `GOG_RUNNER_INVOCATION_TOKEN` — opaque UTF-8 application HMAC key of at
  least 32 bytes.

## Security boundaries

- The ephemeral WIF access token and native ID token are only held in private
  service-to-service requests; they are never returned to a browser, logged,
  embedded in audit rows, or included in error text.
- A small in-memory cache stores only native identity tokens, keyed by issuer,
  provider, key ID, service account, and runner origin, and expires with the
  token. It never caches tenant capabilities or Google user credentials.
- Tenant Google identity is read server-side from the tenant-scoped persisted
  connection. Browser-controlled email/subject hints are never trusted.
- Provider errors, headers, responses, and credentials are reduced to the
  existing safe discovery outcomes: unavailable, error, empty, and ok.
