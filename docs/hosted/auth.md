# Hosted v1 Clerk authentication and tenant bootstrap

The `internal/hosted/app/` package provides the Clerk-protected UI shell for
hosted v1 (#61). It is the first thin slice of the `gog-marketing` Worker:
sign-in/sign-up, server-side identity resolution, and a stable internal
tenant bootstrap. Later tickets (#62–#65) extend this same Worker — they do
not create a duplicate deployment.

## Architecture

### Clerk verification (server-side)

The Worker uses the real `@clerk/backend` SDK (`createClerkClient` +
`authenticateRequest`) to verify session tokens. No custom JWT decoder is
used. The SDK verifies the session JWT signature (via JWKS from Clerk's
Frontend API, or locally via the `CLERK_JWT_KEY` PEM), checks audience and
authorized-party claims, and exposes `requestState.toAuth()` only after
successful verification.

Because `verifyJwt` in `@clerk/backend` 3.22 does **not** enforce the `iss`
claim, the Worker explicitly compares `toAuth().sessionClaims.iss` against
the configured `CLERK_ISSUER` after the SDK's signature verification succeeds.
A token whose `iss` does not exactly match the configured issuer is rejected.
The configured issuer itself must be an `https://` URL on the publishable
key's Frontend API domain; missing, malformed, or mismatched issuer
configuration fails closed with a safe 503.

The authorized-party allowlist (`CLERK_AUTHORIZED_PARTIES`) is an explicit,
non-empty configuration value. Every entry must be an absolute `https://`
origin. It is never overridden with the untrusted request origin, and an
empty or malformed allowlist fails closed with a safe 503.

The verified Clerk user ID is the ONLY identity input. Tenant IDs, user IDs,
model identifiers, and similar fields are never read from request
body/query/header — they are always derived server-side from the verified
Clerk identity.

### Tenant status gating (D1)

After the Clerk session is verified, the Worker resolves the tenant via
`HostedRepository.bootstrapTenant(clerkUserId)`. Only tenants with status
`active` may use the product: requests with a valid session against a
`suspended` or `deleted` tenant receive a safe 403 on both the home page and
the API, before any product content is served. Reactivating the tenant in D1
restores access on the next request.

### Tenant bootstrap (D1)

The `bootstrapTenant` method is the single server-side Clerk→tenant lookup;
it is idempotent (`INSERT ... ON CONFLICT DO NOTHING` + authoritative
reselect) and safe under concurrent first-login callers (all resolve to one
stable tenant).

### Routes

| Route | Auth | Behaviour |
| --- | --- | --- |
| `GET /` | any | Sign-in/sign-up page (real Clerk JS SDK) for unauthenticated visitors; protected product home for authenticated users. |
| `GET /api/tenant` | required | Returns user-safe session status and the verified Clerk user identity only (`{ status, userId }`). The internal tenant UUID is never returned in HTML or API payloads. Returns 401 when unauthenticated and 403 when the tenant is not active. |

The unauthenticated page loads the Clerk SDK (`@clerk/ui@1` +
`@clerk/clerk-js@6`) exactly as documented in the
[Clerk JavaScript quickstart](https://clerk.com/docs/js-frontend/getting-started/quickstart)
and mounts `<SignIn />` with `fallbackRedirectUrl: '/'` so a successful
sign-in redirects to the server-rendered home route.

The signed-in home loads the same Clerk SDK and mounts the real
`<UserButton />` for account management. An explicit **Sign out** button
calls `Clerk.signOut({ redirectUrl: '/' })`, which clears the Clerk session
and returns the user to the unauthenticated home route.

Publishable keys and Clerk domains are validated before they are inserted
into HTML, and all interpolated attribute values are HTML-escaped. HTML
responses carry a baseline `Content-Security-Policy` following the
[official Clerk CSP guide](https://clerk.com/docs/guides/secure/best-practices/csp-headers)
(verified October 5, 2026). Alongside the configured Frontend API domain:

- `script-src` and `frame-src` allow `https://challenges.cloudflare.com` and
  `https://*.protect.clerk.com` for Clerk bot/fraud protection.
- `connect-src` allows `https://*.protect.clerk.com:*`; the port wildcard is
  required because Clerk protection services also use non-443 ports.
- `img-src` includes `https://img.clerk.com` for Clerk-hosted user images.
- `worker-src` is limited to `'self' blob:`.

No bare `*`, broad `https:`/`http:` source, or `unsafe-eval` is added. Existing
same-origin/FAPI allowances, inline loader/runtime styling, `object-src
'none'`, and `frame-ancestors 'none'` remain. Response-level regression tests
assert exact directive source lists on both sign-in and cookie-authenticated
home. Real-browser bot/fraud rendering and CSP-console proof still require
the coordinator's clean-browser acceptance run.

Unknown routes return 404 regardless of authentication state. No generic
HTTP proxy, exec tool, Google Connect, runner, control-plane, or MCP
endpoints are exposed by #61.

### Error safety

Error responses never echo secret keys, raw tokens, authorization headers,
cookie values, or raw Clerk exception text. Unconfigured or misconfigured
environments report `503 Authentication is not configured.`; Clerk network
failures report `502 Authentication service is unavailable.`; unauthenticated
API requests report `401 Authentication required.`; inactive tenants report a
generic `403 Tenant access is not available.`

## Provider configuration

### Clerk application

The existing Clerk application `gog-marketing` (dev instance) is reused. Do
not create a duplicate application. The coordinator acquires the secret key
securely outside the repository and sets it as `CLERK_SECRET_KEY` (a
`wrangler secret put` on the Worker). Secret values never appear in git,
logs, or tool output.

### Non-secret environment variables

These are set on the Cloudflare Worker by the coordinator from the central
provider inventory (`internal/hosted/provider/config.json`) at deploy time:

| Variable | Surface | Purpose |
| --- | --- | --- |
| `CLERK_PUBLISHABLE_KEY` | cloudflare-worker | Public Clerk publishable key identifier |
| `CLERK_ISSUER` | cloudflare-worker | Clerk authorization server issuer URL (validated against the publishable key's Frontend API domain) |
| `CLERK_AUTHORIZED_PARTIES` | cloudflare-worker | Comma-separated origin allowlist for `azp` enforcement (must be non-empty) |

### Secrets

| Variable | Surface | Set via |
| --- | --- | --- |
| `CLERK_SECRET_KEY` | cloudflare-worker | `wrangler secret put CLERK_SECRET_KEY` |

### D1 binding

The Worker binds to the existing `gog-marketing` D1 database (UUID
`328418b4-9649-480f-b26f-5bfb1cd3ffbf`) via the `DB` binding in
`internal/hosted/app/wrangler.toml`. No duplicate database is created.

## Deploy

Deploy commands verified against wrangler 4.147.0 (`wrangler deploy --help`
shows `--var` as a repeated key-value pair; there is no `wrangler vars put`
command):

```sh
cd internal/hosted/app

# 1. Set the Worker secret (value is read from stdin, never logged).
npx wrangler secret put CLERK_SECRET_KEY

# 2. Deploy with the non-secret vars passed explicitly on the command line
#    (public values only, sourced from the central provider inventory).
npx wrangler deploy \
  --var CLERK_PUBLISHABLE_KEY:<publishable-key> \
  --var CLERK_ISSUER:<issuer-url> \
  --var CLERK_AUTHORIZED_PARTIES:<origin-1>,<origin-2>
```

Vars semantics are explicit: by default, `wrangler deploy` deletes any
existing vars that are not part of the deployment configuration, so the three
non-secret Clerk vars must be provided on every deploy (either via `--var` or
via the `[vars]` section of `wrangler.toml`). Use `--keep-vars` only when a
deployment should leave dashboard-set vars untouched. Secrets are never
deleted by deployments.

Before a clean-browser sign-in proof the coordinator must ensure
`CLERK_SECRET_KEY` is set as a Worker secret and the three non-secret
variables are set as Worker vars. Never echo secret values in logs or
terminal output.

## Testing

```sh
cd internal/hosted/app
pnpm install
pnpm test          # vitest run (all route/session/tenant tests)
pnpm typecheck     # tsc --noEmit
pnpm lint          # oxfmt --check + oxlint --deny-warnings
```

Tests use real Clerk SDK verification (`createClerkClient` +
`authenticateRequest`) with a locally generated RSA key pair (networkless
via `jwtKey`) and real Miniflare native D1 for persistence. Tests cover:

- Route protection: unauthenticated requests serve the sign-in page; API
  requests return 401.
- Server-side identity resolution: the server derives the tenant from the
  verified Clerk user ID; the internal tenant UUID never appears in HTML or
  API payloads (the repository/D1 mapping is inspected directly instead).
- Tenant status gating: valid sessions against suspended or deleted tenants
  receive safe 403s on home and API; reactivation restores access.
- Idempotent tenant bootstrap: repeated sign-in resolves to the same tenant;
  concurrent first-login callers resolve to one stable tenant row (native
  D1).
- Issuer/origin enforcement: wrong `iss`, wrong `azp`, missing or malformed
  issuer configuration, and empty or malformed authorized-party allowlists
  all fail closed; expired and tampered tokens are rejected without leaking
  the user ID or secrets.
- Logout wiring: the signed-in home loads the real Clerk SDK, mounts the
  documented `<UserButton />`, and wires sign-out to
  `Clerk.signOut({ redirectUrl: '/' })`. Real-browser logout proof is owned
  by the coordinator (no synthetic browser proof is claimed here).
- Error safety: responses never echo secret keys, tokens, or authorization
  headers.
