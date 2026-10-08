# Hosted v1 operator runbook

This runbook covers the deployed hosted v1 owner journey for
`Rajeev-SG/gog-marketing`. It records command shapes and configuration names
only. Never print, paste, commit, screenshot, or log secret values, OAuth
codes, access/refresh tokens, Clerk secret keys, credential-encryption keys,
Google credentials, or raw provider responses.

## Deployment inventory

Use the existing provider resources:

| Resource | Name |
| --- | --- |
| Cloudflare Worker | `gog-marketing` |
| Cloudflare D1 database | `gog-marketing` |
| GCP project | `gog-marketing-prod` |
| Artifact Registry repository | `gog-marketing` in `europe-west2` |
| Cloud Run service | `gog-marketing-runner` in `europe-west2` |
| Cloud Run runner identity | `gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com` |

### Worker variables

Set these non-secret variables on the Worker. Do not put their values in this
repository or in command output.

| Name | Purpose |
| --- | --- |
| `CLERK_PUBLISHABLE_KEY` | Clerk publishable key identifier |
| `CLERK_ISSUER` | Clerk authorization-server issuer URL |
| `CLERK_AUTHORIZED_PARTIES` | Comma-separated trusted HTTPS origins |
| `GOG_GOOGLE_OAUTH_CLIENT_ID` | Google OAuth public client identifier |
| `GOG_GOOGLE_OAUTH_REDIRECT_URI` | Canonical hosted Google callback URI |
| `GOG_HOSTED_CANONICAL_ORIGIN` | Canonical HTTPS origin; `/mcp` is appended by the app |
| `GOG_CLOUD_RUN_SERVICE_URL` | Private Cloud Run runner HTTPS `/v1/execute` URL |
| `GOG_RUNNER_WIF_PROVIDER` | Workload Identity Federation provider resource |
| `GOG_RUNNER_WIF_ISSUER` | Fixed trusted Worker issuer |
| `GOG_RUNNER_WIF_KEY_ID` | Workload signing-key identifier |
| `GOG_RUNNER_SERVICE_ACCOUNT` | Runner Google service account |
| `GOG_HOSTED_OAUTH_STATE_TTL_SECONDS` | OAuth state lifetime; default `600` seconds |
| `GOG_MCP_TOOL_CALL_DAILY_LIMIT` | Per-tenant daily `tools/call` quota; default `100` |
| `GOG_MCP_REQUESTS_PER_MINUTE_LIMIT` | Per-tenant and hashed-IP request limit; default `60` |

`GOG_GOOGLE_OAUTH_AUTH_URL`, `GOG_GOOGLE_OAUTH_TOKEN_URL`,
`GOG_GOOGLE_OAUTH_USERINFO_URL`, and `GOG_GOOGLE_OAUTH_REVOKE_URL` are
test/staging endpoint overrides. Keep them unset in production.

### Worker secrets

Provision these with `wrangler secret put NAME`; the command reads the value
from stdin and must not echo it:

| Name | Purpose |
| --- | --- |
| `CLERK_SECRET_KEY` | Clerk server-side secret |
| `GOG_GOOGLE_OAUTH_CLIENT_SECRET` | Google OAuth client secret |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY` | Credential-encryption root key |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS` | Optional rotation map by key version |
| `GOG_RUNNER_WIF_SIGNING_KEY` | Worker-to-Google WIF signing key |
| `GOG_RUNNER_INVOCATION_TOKEN` | Worker-to-runner application HMAC key; at least 32 bytes |

`GOG_RUNNER_INVOCATION_TOKEN` must be the same secret material held by the
Cloud Run runner through Secret Manager. Do not place it in an image, source
file, shell history, log, audit row, or screenshot.

### Cloud Run runner variables and secrets

The runner reads:

| Name | Default | Notes |
| --- | --- | --- |
| `PORT` | `8080` | Set by Cloud Run/container runtime |
| `GOG_RUNNER_TIMEOUT` | `20s` | Valid Go duration, maximum `1m` |
| `GOG_RUNNER_MAX_REQUEST_BYTES` | `65536` | Positive integer |
| `GOG_RUNNER_MAX_RESPONSE_BYTES` | `262144` | Positive integer |
| `GOG_RUNNER_INVOCATION_TOKEN` | none | Required secret, at least 32 bytes |

`GOG_RUNNER_INVOCATION_TOKEN` is the only runner secret. The runner receives
short-lived Google credentials only inside a private request and never stores
or returns them.

## Deploy the Worker

Run from the repository root. Use values from the operator's protected
provider inventory without printing them:

```sh
cd internal/hosted/app

npx wrangler secret put CLERK_SECRET_KEY
npx wrangler secret put GOG_GOOGLE_OAUTH_CLIENT_SECRET
npx wrangler secret put GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY
npx wrangler secret put GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS   # only during key rotation
npx wrangler secret put GOG_RUNNER_WIF_SIGNING_KEY
npx wrangler secret put GOG_RUNNER_INVOCATION_TOKEN

npx wrangler deploy \
  --var "CLERK_PUBLISHABLE_KEY:${CLERK_PUBLISHABLE_KEY}" \
  --var "CLERK_ISSUER:${CLERK_ISSUER}" \
  --var "CLERK_AUTHORIZED_PARTIES:${CLERK_AUTHORIZED_PARTIES}" \
  --var "GOG_GOOGLE_OAUTH_CLIENT_ID:${GOG_GOOGLE_OAUTH_CLIENT_ID}" \
  --var "GOG_GOOGLE_OAUTH_REDIRECT_URI:${GOG_GOOGLE_OAUTH_REDIRECT_URI}" \
  --var "GOG_HOSTED_CANONICAL_ORIGIN:${GOG_HOSTED_CANONICAL_ORIGIN}" \
  --var "GOG_CLOUD_RUN_SERVICE_URL:${GOG_CLOUD_RUN_SERVICE_URL}" \
  --var "GOG_RUNNER_WIF_PROVIDER:${GOG_RUNNER_WIF_PROVIDER}" \
  --var "GOG_RUNNER_WIF_ISSUER:${GOG_RUNNER_WIF_ISSUER}" \
  --var "GOG_RUNNER_WIF_KEY_ID:${GOG_RUNNER_WIF_KEY_ID}" \
  --var "GOG_RUNNER_SERVICE_ACCOUNT:${GOG_RUNNER_SERVICE_ACCOUNT}" \
  --var "GOG_HOSTED_OAUTH_STATE_TTL_SECONDS:${GOG_HOSTED_OAUTH_STATE_TTL_SECONDS}" \
  --var "GOG_MCP_TOOL_CALL_DAILY_LIMIT:${GOG_MCP_TOOL_CALL_DAILY_LIMIT}" \
  --var "GOG_MCP_REQUESTS_PER_MINUTE_LIMIT:${GOG_MCP_REQUESTS_PER_MINUTE_LIMIT}"
```

Wrangler removes vars omitted from a deployment configuration. Supply the
complete non-secret set on every deploy, or deliberately use `--keep-vars`
after reviewing that decision. Secrets are not deleted by deploys. Verify the
names without values:

```sh
npx wrangler secret list --name gog-marketing
npx wrangler deployments status --name gog-marketing --json
```

## Apply D1 migrations

The four migrations are additive, but they are not all safe to re-run:
`0002` includes unconditional `ALTER TABLE ADD COLUMN` statements. Apply each
migration at most once to the existing `gog-marketing` database; do not create
another database or execute migration files directly with
`wrangler d1 execute`. Run from `internal/hosted/state`.

Wrangler compares the migration filenames with the `d1_migrations` tracking
table. `migrations list` reports only the pending files, and `migrations apply`
applies those files in filename order and records each successful filename.
Use `set -e` so the procedure stops at the first command error.

```sh
set -e

# Review the pending set first; an empty result means no migration is needed.
npx wrangler d1 migrations list gog-marketing \
  --remote \
  --config wrangler.migrations.toml

# Apply only the pending migrations in filename order.
npx wrangler d1 migrations apply gog-marketing \
  --remote \
  --config wrangler.migrations.toml
```

Before executing remotely, confirm that the `database_name` and `database_id`
in `wrangler.migrations.toml` refer to the existing intended database. These
files intentionally contain no destructive statements. Do not run ad-hoc
`DROP`, `DELETE`, or credential-table mutations as part of deployment.

## Build, deploy, and roll back the Cloud Run runner

Set a new immutable image tag; do not rely on `latest` for rollback evidence.

```sh
cd /path/to/gog-marketing

IMAGE="europe-west2-docker.pkg.dev/gog-marketing-prod/gog-marketing/gog-marketing-runner:${RUNNER_IMAGE_TAG}"

gcloud auth configure-docker europe-west2-docker.pkg.dev
docker build --platform linux/amd64 \
  --file Dockerfile.hosted-runner \
  --tag "$IMAGE" \
  .
docker push "$IMAGE"

gcloud run deploy gog-marketing-runner \
  --project gog-marketing-prod \
  --region europe-west2 \
  --image "$IMAGE" \
  --no-allow-unauthenticated \
  --service-account gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com \
  --set-secrets "GOG_RUNNER_INVOCATION_TOKEN=${RUNNER_SECRET_RESOURCE}:latest" \
  --min-instances 0 \
  --max-instances 20 \
  --port 8080
```

Confirm the deployed revision and that the service remains private:

```sh
gcloud run services describe gog-marketing-runner \
  --project gog-marketing-prod \
  --region europe-west2 \
  --format=yaml
```

The live service must retain request-based billing, minimum instances `0`, the
existing CPU/memory/concurrency/timeout policy, and `--no-allow-unauthenticated`.
The maximum instance setting is a safety ceiling, not reserved capacity.

To roll back the runner, list revisions newest first and choose the last known
good revision recorded during acceptance:

```sh
gcloud run revisions list \
  --project gog-marketing-prod \
  --region europe-west2 \
  --service gog-marketing-runner \
  --sort-by='~metadata.creationTimestamp' \
  --limit 10

gcloud run services update-traffic gog-marketing-runner \
  --project gog-marketing-prod \
  --region europe-west2 \
  --to-revisions "${KNOWN_GOOD_RUNNER_REVISION}=100"
```

Rollback changes traffic only; it does not delete the bad revision or alter
Secret Manager versions.

## Local development

Use local-only D1 and non-production Clerk/Google configuration. Do not import
the production owner credential into a development process. Set
`LOCAL_PERSIST_DIR` once to one absolute directory and use that same explicit
directory for both commands. The migrations and Worker commands run from
different package directories, so their default `.wrangler/state` directories
would otherwise diverge and leave the app using an unmigrated database.

```sh
LOCAL_PERSIST_DIR="/path/to/gog-marketing/internal/hosted/.wrangler/state"

pnpm -C internal/hosted/state install --frozen-lockfile
(
  cd internal/hosted/state
  npx wrangler d1 migrations apply gog-marketing \
    --local \
    --config wrangler.migrations.toml \
    --persist-to "$LOCAL_PERSIST_DIR"
)

pnpm -C internal/hosted/app install --frozen-lockfile
(
  cd internal/hosted/app
  npx wrangler dev --local --persist-to "$LOCAL_PERSIST_DIR"
)
```

Provide the Worker variable and secret names from the inventory through the
local Wrangler development configuration. Do not commit `.dev.vars`, local
tokens, or test credentials.

Run the deterministic suites with the repository targets:

```sh
make state-ci
make hosted-app-ci
```

Run the Playwright deterministic subset from
`internal/hosted/app/e2e` after installing its package:

```sh
pnpm install --frozen-lockfile
pnpm install:browsers
pnpm test:deterministic
```

When `PLAYWRIGHT_BASE_URL` is unset, Playwright starts its local Worker fixture.
When it is set, the suite tests that base URL.

## Live owner-journey acceptance

Perform this only from a clean browser with the coordinator's real owner
account. Do not paste credentials or tokens into terminal output.

1. Record the deployed source:

   ```sh
   git rev-parse HEAD
   git status --short
   npx --yes wrangler --version
   gcloud --version
   docker --version
   node --version
   pnpm --version
   ```

2. Load the public home URL. Confirm that unauthenticated visitors receive the
   Clerk sign-in entry point and that API requests without a session receive
   `401`.
3. Sign in through Clerk. Connect the real Google account with read-only
   consent and confirm the connect-account section renders.
4. Discover a real resource, enable exactly one resource, save, reload, and
   confirm persistence.
5. Copy the canonical MCP URL from the workspace and configure Codex with that
   URL. Complete MCP OAuth through Clerk. Do not paste an OAuth token.
6. Confirm Codex lists only permitted tools. Record the actual tool used below.
7. Invoke a real read against the enabled resource and record `allow`.
8. Disable that resource, retry, and record `deny`. Re-enable it, retry, and
   record `allow`.
9. Restart or redeploy the Worker and the Cloud Run runner, repeat the read,
   and confirm the selection and outcomes persist.
10. Search browser output, Playwright artifacts, MCP output, Cloudflare logs,
    Cloud Run logs, and D1 audit rows for accidental OAuth codes, access or
    refresh tokens, Clerk secret keys, encryption keys, Google credentials,
    and provider access tokens. No such value may be present.

Run the real Clerk owner UI smoke test only after the browser is installed:

```sh
cd internal/hosted/app/e2e
pnpm install --frozen-lockfile
pnpm install:browsers
PLAYWRIGHT_BASE_URL="${PUBLIC_ORIGIN}" \
TEST_CLERK_EMAIL="${TEST_CLERK_EMAIL}" \
TEST_CLERK_PASSWORD="${TEST_CLERK_PASSWORD}" \
pnpm test:owner
```

The owner test skips unless both `TEST_CLERK_EMAIL` and
`TEST_CLERK_PASSWORD` are set. Its Google asset persistence calls are mocked;
the real Google connection, discovery, MCP OAuth, and provider read remain
manual live acceptance steps above.

### MCP acceptance probes

Export only the already-authorized access token and test identifiers into the
operator shell. Do not echo them.

```sh
PUBLIC_ORIGIN="${GOG_HOSTED_CANONICAL_ORIGIN:?set GOG_HOSTED_CANONICAL_ORIGIN}"
MCP_URL="${PUBLIC_ORIGIN}/mcp"
MCP_PROTOCOL_VERSION="2025-11-25"
MCP_HEADERS=(-H "Authorization: Bearer ${MCP_ACCESS_TOKEN:?set MCP_ACCESS_TOKEN}"
  -H "Accept: application/json, text/event-stream"
  -H "Content-Type: application/json")
```

Metadata routes:

```sh
curl --silent --show-error --fail-with-body \
  "${PUBLIC_ORIGIN}/.well-known/oauth-protected-resource"

curl --silent --show-error --fail-with-body \
  "${PUBLIC_ORIGIN}/.well-known/oauth-authorization-server"
```

Initialize:

```sh
curl --silent --show-error --fail-with-body "${MCP_HEADERS[@]}" "$MCP_URL" \
  --data-binary @- <<'JSON'
{"jsonrpc":"2.0","id":"initialize-1","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"operator-acceptance","version":"1"}}}
JSON
```

List permitted tools:

```sh
curl --silent --show-error --fail-with-body "${MCP_HEADERS[@]}" "$MCP_URL" \
  --data-binary '{"jsonrpc":"2.0","id":"tools-list-1","method":"tools/list","params":{}}'
```

Allowed read:

```sh
curl --silent --show-error --fail-with-body "${MCP_HEADERS[@]}" "$MCP_URL" \
  --data-binary @- <<'JSON'
{"jsonrpc":"2.0","id":"allow-1","method":"tools/call","params":{"name":"analytics_properties_get","arguments":{"property":"TEST_ENABLED_PROPERTY"}}}
JSON
```

Disabled-resource and foreign-connection denials:

```sh
curl --silent --show-error --fail-with-body "${MCP_HEADERS[@]}" "$MCP_URL" \
  --data-binary @- <<'JSON'
{"jsonrpc":"2.0","id":"deny-disabled-1","method":"tools/call","params":{"name":"analytics_properties_get","arguments":{"property":"TEST_DISABLED_PROPERTY"}}}
JSON

curl --silent --show-error --fail-with-body "${MCP_HEADERS[@]}" "$MCP_URL" \
  --data-binary @- <<'JSON'
{"jsonrpc":"2.0","id":"deny-foreign-1","method":"tools/call","params":{"name":"analytics_properties_get","connectionId":"TEST_FOREIGN_CONNECTION_ID","arguments":{"property":"TEST_ENABLED_PROPERTY"}}}
JSON
```

Expected outcome classes:

| Probe | Expected |
| --- | --- |
| Missing/invalid bearer | HTTP `401`, `-32001`, `WWW-Authenticate` challenge |
| Tenant/IP request limit | HTTP `429`, `-32029`, `retryAfterSeconds` |
| Daily tool-call quota | HTTP `429`, `-32003`, `quota_exceeded` |
| Unknown/foreign connection or resource | policy denial; no runner call |
| Enabled read | HTTP `200`, tool result and `allow` audit |
| Disabled read | policy denial and `deny` audit |

Record the acceptance result with public identifiers redacted:

```text
commit SHA:
public origin:
metadata endpoints:
MCP endpoint:
tool name:
enabled resource reference (redacted):
allow result:
deny result after disable:
allow result after re-enable:
Worker deployment ID:
Cloud Run revision:
Wrangler version:
Google Cloud CLI version:
Docker version:
```

## Quota, abuse, and observability

The authoritative settings and response semantics are in
[`observability.md`](observability.md):

| Name | Default | Window |
| --- | ---: | --- |
| `GOG_MCP_TOOL_CALL_DAILY_LIMIT` | `100` | UTC calendar day per tenant |
| `GOG_MCP_REQUESTS_PER_MINUTE_LIMIT` | `60` | UTC fixed minute per tenant and hashed IP |

Both values must be positive integers. Invalid values fail `/mcp` closed.
`tools/call` consumes quota before policy execution. Rate exhaustion is
JSON-RPC `-32029`; quota exhaustion is `-32003`; counter failures are `-32603`
and execute no uncounted tool call.

Use the aggregate, secret-free SQL queries in
[`observability.md`](observability.md). The command shape is:

```sh
cd internal/hosted/state
npx wrangler d1 execute gog-marketing \
  --remote \
  --config wrangler.migrations.toml \
  --json \
  --command '<READ-ONLY SQL FROM observability.md>'
```

Alert on rising `error`, `quota_exceeded`, `rate_limited`,
`provider_unavailable`, `needs_reconnect`, or `internal_error` counts and on
approach to the limits below. Never query raw request bodies, bearer values,
credential ciphertext, Google emails, or source IPs into an incident channel.

## Rollback procedure

1. Stop new acceptance or rollout activity and record the current Worker
   version ID and Cloud Run revision. Before rollout, record the known-good
   Worker version ID from the deploy output as
   `KNOWN_GOOD_WORKER_VERSION_ID`. Do not substitute a deployment ID: the
   `wrangler rollback` command requires a Worker version ID.
2. Require that recorded version ID, then roll back the Worker:

   ```sh
   cd internal/hosted/app
   : "${KNOWN_GOOD_WORKER_VERSION_ID:?record it from the deploy output}"
   npx wrangler rollback "${KNOWN_GOOD_WORKER_VERSION_ID}" --name gog-marketing
   ```

3. Roll back the runner with the traffic command above. Keep minimum instances
   `0`; rollback must not create an always-on billable instance.
4. Do not automatically reverse D1 migrations. `0001`–`0004` are additive and
   compatible with the previous Worker. Roll application code back first and
   retain the additive schema. Any destructive schema restoration is a
   separate reviewed recovery operation using a verified backup.
5. Re-run metadata, `initialize`, `tools/list`, one allowed read, and one
   disabled-resource denial. Confirm persisted resource selection.
6. Inspect the observability queries and the secret-leak checklist. Record the
   final deployment IDs and outcomes.

## £0/free-tier assumptions

The authoritative source and current provider links are in
[`free-tier-envelope.md`](free-tier-envelope.md). The operating assumptions
are:

| Surface | £0 assumption | Operator action |
| --- | --- | --- |
| Cloudflare Workers | Free request and CPU envelope is sufficient | Monitor request count and CPU |
| Cloudflare D1 | Free read/write rows and storage are sufficient | Watch quota, rate, and audit write pressure |
| Cloudflare KV | Not used by the hosted v1 state path | Do not move authoritative state to KV |
| Cloud Run | Request-based billing, minimum instances `0`, maximum `20` | Keep minimum instances `0`; review request/CPU growth |
| Google APIs | Existing OAuth/API access has no new paid hosted-v1 dependency | Re-check provider terms before capacity changes |

There is no fixed monthly spend from the Cloud Run maximum-instance setting.
The first expected paid bottleneck is Cloud Run request/CPU usage; the first
hard failures may instead be Cloudflare Workers or D1 daily limits. Re-check
all provider pricing and limits before admitting the full beta envelope.
