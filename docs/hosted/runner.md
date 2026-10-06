# Hosted v1 private execution runner (#63)

The Cloud Run runner is a small private HTTP service that packages the
existing typed Go/gog Google engine behind one fixed endpoint. It is not a
general command surface: there is no CLI spawn, shell, argv, arbitrary HTTP
proxy, or `gog_exec` escape hatch. Only the two operations documented below
are executable, and every request is rejected before any Google engine work
unless it carries a valid worker capability.

Real Cloud Run and private Google acceptance (IAM, workload identity,
actual permitted GA4 reads) is coordinator-owned. The tests in
`internal/hosted/runner` prove the protocol and guarded engine delegation;
they are development evidence only, never final cloud acceptance.

## Container and deployment contract

- Build context: repository root. `Dockerfile.hosted-runner` follows the
  existing pinned Go builder conventions (`golang:1.27.1-alpine` build,
  `alpine:3.24` runtime) and runs as non-root user `gog` (uid 10001).
- Binary path: `cmd/hosted-runner`; package logic lives in
  `internal/hosted/runner`.
- `PORT` defaults to `8080`; the process binds only `:$PORT`.
- Required secret: `GOG_RUNNER_INVOCATION_TOKEN` — at least 32 bytes of
  high-entropy signing material dedicated to this service. It is **not** a
  Google credential, service-account key, or CLI root. The runner exits if it
  is missing or too short.
- Optional env: `GOG_RUNNER_TIMEOUT` (default `20s`, max `1m`),
  `GOG_RUNNER_MAX_REQUEST_BYTES` (default `65536`),
  `GOG_RUNNER_MAX_RESPONSE_BYTES` (default `262144`).
- Deployment (coordinator-owned): Cloud Run with `min instances 0`
  (scale-to-zero) and a bounded `max instances` (recommended `2` for the v1
  worker), invocation authentication enabled. Never deploy with
  `--no-allow-unauthenticated`; anonymous traffic must be rejected by the
  native IAM admission gate before the process is reached. The capability
  JWT below is the application-level second gate.
- The Worker may send the native Google ID token on
  `X-Serverless-Authorization` for the IAM gate while continuing to send the
  capability JWT on `Authorization`. The runner only reads
  `Authorization`.

## Routes

- `GET /healthz` — unauthenticated container probe. Returns
  `{"status":"ok","executions":N}` with no tenant data.
- `POST /v1/execute` — the only execute route. Everything else is 404.

## Capability authentication

`Authorization: Bearer <token>` must be a compact HS256 JWT signed with
`GOG_RUNNER_INVOCATION_TOKEN` (stdlib-style; the later Worker bridge produces
it with `Web Crypto`). Fixed header: `{"alg":"HS256"}` only. Payload claims:

```json
{
  "iss": "gog-marketing-worker",
  "sub": "gog-marketing-worker",
  "aud": "gog-marketing-runner",
  "iat": 1780000000,
  "exp": 1780000030,
  "tenant_id": "<hosted tenant id>",
  "connection_id": "<hosted connection id>",
  "operation": "discover | analytics_property_read",
  "request_sha256": "<lowercase hex sha256 of the exact request body bytes>"
}
```

The signer hashes the exact JSON body bytes that will be sent. The runner
verifies signature, exact algorithm, issuer/subject/audience, an integer
finite window with `exp - iat <= 60` seconds and `iat <= now < exp`, and the
body digest — all before decoding the body. It then independently matches
`tenant_id`, `connection_id`, and `operation` claims to the decoded body, so a
correctly signed token with mismatched context is rejected (403
`context_mismatch`). Rejection codes are stable: `anonymous`,
`bad_signature`, `token_invalid`, `token_expired`, `token_not_yet_valid`,
`request_digest_mismatch` (all 401), `context_mismatch` (403),
`invalid_request` (400), `timeout` (504), `output_limit` (422),
`engine_error` (502), `server_config` (500). Error messages are fixed safe
strings; provider exceptions, credentials, and emails never appear in
responses or logs.

## Request schema (strict JSON)

```json
{
  "request_id": "optional [A-Za-z0-9._-]{1,64}",
  "tenant_id": "<hosted tenant id>",
  "connection_id": "<hosted connection id>",
  "operation": "discover",
  "google_email": "<connection Google account email>",
  "google_subject": "optional Google subject id",
  "access_token": "<ephemeral Google access token>",
  "services": ["analytics", "googleads"],
  "resource": null
}
```

For `analytics_property_read`, `operation`, and `resource` become:

```json
{
  "operation": "analytics_property_read",
  "resource": {
    "service": "analytics",
    "resource_type": "property",
    "resource_id": "properties/<digits>",
    "enabled": true
  }
}
```

Rules enforced before any Google call: strict bounded JSON with unknown
fields and trailing values rejected; sizes and types validated; `access_token`
is accepted as ephemeral material only and is never persisted, refreshed, or
fed to any credential store; no refresh-token path, ADC, or keyring fallback
exists. The capability authenticates a trusted control-plane snapshot —
tenant, connection, grant enablement, and Google account come from the worker,
not from caller-asserted hints. Reads require an enabled analytics/property
grant with a canonical `properties/<digits>` resource id.

## Operations

### discover

Reuses `controlplane.EngineDiscoverer.DiscoverReport` unchanged. `services`
must be a deduplicated list of 1–10 established canonical service names:
`analytics`, `tagmanager`, `googleads`, `searchconsole`, `bigquery`. Anything
else is rejected before the engine. Operator-dependent services stay honest:
for example, Google Ads without a configured developer token returns an
`unavailable` status with detail `google_ads_unconfigured` — never fabricated
resources. There is no BigQuery query execution surface anywhere in this
service; BigQuery discovery lists projects/datasets only, and query costs are
out of scope by design.

Success response (`ok: true`):

```json
{
  "request_id": "req-…",
  "operation": "discover",
  "ok": true,
  "result": {
    "resources": [
      {
        "service": "analytics",
        "resource_type": "property",
        "resource_id": "properties/123",
        "display_name": "…",
        "parent": "accounts/…",
        "enabled": false
      }
    ],
    "statuses": {
      "analytics": {"state": "ok", "resource_count": 1, "checked_at": "…"},
      "googleads": {"state": "unavailable", "detail": "google_ads_unconfigured", "checked_at": "…"}
    }
  },
  "duration_ms": 12
}
```

### analytics_property_read

One real typed guarded Google Analytics Admin property GET
(`service.Properties.Get`), executed through the existing
`googleapi`/`authclient` stored-token contract with read-only and no-input
context flags. The whitelisted result never copies the upstream schema:

```json
{
  "operation": "analytics_property_read",
  "ok": true,
  "result": {
    "name": "properties/123",
    "display_name": "…",
    "time_zone": "…",
    "currency_code": "…"
  }
}
```

Every operation runs inside the finite deadline (default 20s) with request and
output limits; oversized results return `output_limit` without leaking the
payload.

## Audit and logging

One small structured line is logged per request (successes and rejections):
`request_id`, `operation`, `tenant_hash`, `connection_hash`, `outcome`,
`duration_ms`, and a cumulative `executions` count. Tenant and connection
identifiers are hashed; request bodies, authorization headers, credentials,
Google emails, and raw provider exceptions are never logged. The `Counter`
and engine seams are injectable so tests can prove rejection counting and
before-engine behavior without faking production responses.
