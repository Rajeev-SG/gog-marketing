# Hosted v1 private execution runner (#63)

The Cloud Run runner is a small private HTTP service that packages the
existing typed Go/gog Google engine behind one fixed endpoint. It is not a
general command surface: there is no CLI spawn, shell, argv, arbitrary HTTP
proxy, or `gog_exec` escape hatch. Only the typed operations documented below
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
  worker), with `--no-allow-unauthenticated`. Never deploy with
  `--allow-unauthenticated` or `--no-invoker-iam-check`; anonymous traffic
  must be rejected by the native IAM admission gate before the process is
  reached. The capability JWT below is the application-level second gate.
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
  "operation": "discover | analytics_property_read | tool_call | verify_files",
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
  "services": ["analytics", "tagmanager"],
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
`analytics`, `tagmanager`, `searchconsole`, `bigquery`. Anything
else is rejected before the engine. Excluded Gmail, Google Ads and Drive Activity are rejected before execution.
The existing local discovery engine is unchanged. There is no BigQuery query execution surface anywhere in this
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
      "tagmanager": {"state": "ok", "resource_count": 0, "checked_at": "…"}
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

## Signed native tool calls (#52 / #79)

`operation: "tool_call"` adds a `tool` object with `name`, `arguments` and
`authorization`. Authorization is a trusted Worker snapshot, **not model input**:
`service`, explicit `allowed_tools`, actual granted `scopes`, one enabled canonical
`resource`, `allow_write`, and `allow_destructive`. Google subject is mandatory.
Validation rejects missing/disabled tool/resource/write authority, excluded or
unknown/transitive scopes, insufficient method scopes and mismatched resource IDs
before native handler execution. The existing MCP SDK validates typed arguments;
no shell, subprocess, arbitrary command or HTTP proxy is exposed.

The executable entrypoint injects the existing typed command executor through
`internal/hosted/native`; the runner package does not import the command layer.
Missing executor wiring fails closed. The catalogue executable composes the same
native schemas with runner bindings; its generated Worker drift check remains
mandatory.

Initial guarded bindings: GA4 property get/report/data-stream list; selected-file
Drive get, Docs get/write, Sheets read/update; selected Calendar events. Nine
private bindings were the initial coverage checkpoint. The Worker routes the
generated executable catalogue only when account tool and matching resource
grants are enabled. New Workspace writes are not seeded by migration or implied
by OAuth consent; Picker's production prerequisites remain unverified.
Neither code routing nor fixture execution proves deployed acceptance.
The full eligible 27-service surface remains required by #79.

Additional Slides bindings reuse `slides list-slides` and `slides update-notes`:
`slides_list_slides` reads one selected presentation; `slides_update_notes`
replaces speaker notes only with explicit write and destructive grants. Their
guard permits exact presentation GET and batchUpdate paths; neither tool accepts
local files or arbitrary API requests. Eleven bindings across six services still
do not establish full parity or live acceptance.

Account-level bindings have an empty resource model and reject a supplied
resource grant rather than manufacturing an asset. `tasks_lists` and
`tasks_lists_create` use the existing Tasks handlers and admit only GET or POST,
respectively, on `tasks.googleapis.com/tasks/v1/users/@me/lists`. Creation needs
the `tasks` scope and explicit write authority; `tasks.readonly` cannot mutate.
Their Worker authorization resolves an owned active connection and its exact
enabled tool grant; ambiguous eligible accounts require account selection.
All existing resource-level bindings still require enabled matching resources.

Tasks now also binds `tasks_list`, `tasks_get`, `tasks_raw`, `tasks_add`,
`tasks_update`, `tasks_done`, `tasks_undo`, `tasks_delete` and `tasks_clear`.
Selectors must be opaque task-list IDs (at least 16 characters) or `@default`,
not list-name resolution; task IDs are validated before native execution. Guards
admit only the matching list/task path and method. Delete and clearing completed
tasks require separate destructive grants. Listing supports native filters,
pagination and all-pages options. Repeated creation requires an explicit
`repeat_count` of 1–1000, even with `repeat_until`; it is not transactional and
earlier creations may persist after a later provider failure. Use a read-back,
not a blind retry. Local CLI repetition behavior is unchanged.

**Resolved native serialization gap (12 October):** the existing Tasks updater
now uses SDK `ForceSendFields` for explicitly empty title/notes values. Omitted
fields remain untouched, and due-date clearing keeps native `NullFields` behavior.
Hosted schemas document these clears. Status is restricted to Google's documented
`needsAction`/`completed` values; use `tasks_undo` to reset it. Blank status is not
advertised as a working API capability. Native payload and fixture read-back tests
pass, but real Google clearing remains untested.

`tagmanager_versions_publish` reuses the existing local MCP argument builder and
native publishing handler. Hosted input replaces separate account/container IDs
with `container_path` (`accounts/123/containers/456`), matching the canonical
container grant from existing discovery. The selected connection must enable
that container and exact tool, have `tagmanager.publish`, and explicitly permit
both writes and destructive operations. Admission permits only POST to the
selected numeric version's `:publish` path on `tagmanager.googleapis.com`;
foreign containers and alternate methods cannot pass. Fingerprint input retains
the native optimistic concurrency safeguard. HTTP 200 with `compilerError: true`
is an execution error, not a publication-success signal. No live publication or
live-version read-back has been verified.

GA4 Admin data-stream/key-event families now expose list/get/create/update/delete
through fixed schemas and existing native handlers. All use `property` as the
enabled resource selector; child operations additionally take `resource_id`.
Read methods permit `analytics.readonly` or `analytics.edit`; mutations require
`analytics.edit`, and deletes require explicit destructive permission. Wildcard
or full-replacement update masks also require destructive permission. Guards
admit only the family's exact selected-property/child path and method. Bodies
must be inline JSON objects (`body_json`); `@file`, arrays, null and foreign
resource names are rejected before native execution. Required snake_case update masks and
list pagination/all-pages controls pass to existing handlers. This does not
expose Google Ads links or authorize an indirect `adwords` scope. Fixture CRUD
and read-back evidence is not real Google acceptance.

Search Console now registers site list/get, search analytics query, sitemap
list/get/submit/delete and URL inspection using existing typed tools/handlers.
Site listing uses an explicit account tool grant; the other tools require an
enabled `site` resource (`site_url`), supporting URL-prefix or `sc-domain:`
properties. Read methods accept `webmasters.readonly` or `webmasters`; sitemap
mutations require `webmasters`, and deletion requires destructive permission.
Search Analytics and inspection POSTs are semantically read-only and admitted
only for their declared exact endpoints. Site/feed URL path segments must match
the existing SDK's escaped encoding, not an ambiguous decoded path.

Inspection and sitemap URLs must belong to the enabled URL-prefix/domain scope;
suffix lookalikes, foreign hosts and traversal are rejected before Google calls.
Cross-site sitemap hosting requires additional resource authorization and is
not currently exposed by this single-site snapshot. Richer native query/filter
options remain to be wired. Fixture submit/read/delete persistence, domain-site
support and denials do not prove real Search Console acceptance.

Contacts list/search/get/create/update/delete now reuse the existing People API
handlers with account-level tool grants. Reads need `contacts.readonly` or
`contacts`; mutations need `contacts`. Explicit `people/...` IDs are required for
get/update/delete rather than email/name resolution. Native create/update scalar
and list fields pass through fixed schemas; update preserves native source/person
etags and field masks. Empty list inputs use native clearing semantics. The guard
permits only declared People endpoints and contact paths, including GET then PATCH
for update, and deletion requires destructive permission. Search retains gog's
five-second cache warmup inside the normal 20-second hosted deadline and caps
results at Google's published maximum of 30. No file input or ignore-etag bypass
is exposed. Batch, dedupe/merge, export, directory/other reads and file-I/O adapters
remain engineering gaps; fixture evidence does not prove live Contacts behavior.

Calendar resource discovery now calls the existing typed Calendar client, follows
all CalendarList pages and deduplicates IDs, excluding deleted entries. A later
page failure or repeated token produces an error without a partial inventory.
New grants remain disabled; rediscovery retains prior user choices. Discovery
admits only the fixed CalendarList GET endpoint, and plus/hash IDs such as
holiday calendars are accepted by resource matching.

**Known blocker discovered by execution:** `calendar_events` currently asks the
native command runtime for local alias configuration before creating a Google
client. Hosted execution correctly denies this ambient-config access. Calendar
discovery/selection is implemented, but event execution is not working yet;
its explicit-ID/config-free native path and write coverage remain required.

The hosted request guard is opt-in in the existing Google transport. It admits
only the binding's exact Google host, method and resource path, rejecting foreign
files, unexpected endpoints/redirects, method overrides and token query strings.
It carries the operation deadline even when native SDK handlers omit `Context`,
keeps response bodies alive until consumption/close, and preserves the ordinary
CLI transport when no hosted guard exists. Do not remove native destructive
confirmation or use model-supplied force; document replacement needs signed
`allow_destructive` authority. Cross-file Sheets imports are not implied by a
single selected-file write grant.

The exact consent scope allowlist is shared in
`internal/hosted/policy/services.json`; local CLI scopes remain untouched.
Transport/runner fixture tests are development evidence, not deployed Google
write, Picker, Clerk/Codex, account-isolation or public-beta acceptance.

### verify_files

The signed `verify_files` operation takes `file_selection: {ids, scopes}` with
1–20 distinct opaque IDs and the actual audit-free grant containing `drive.file`.
Google subject is mandatory. Existing typed `googleapi.NewDrive` performs metadata
GETs for exactly those IDs; the hosted transport denies other hosts/paths/methods.
The result contains Google-authoritative `{id,name,mime_type}` files and rejected
IDs, never provider exception text. Trashed files, shortcuts and returned-ID
mismatches are rejected. Partial failures are explicit. This privileged UI
onboarding operation is not exposed to MCP clients; its authority comes from a
one-use account/Clerk-session-bound Picker flow in the Worker.
