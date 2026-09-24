# Build gog-marketing v1: GA4 Admin, GTM, Google Ads, Search Console and BigQuery

## Goal

Turn this fork into **gog for the Google marketing/data stack**: one smooth multi-account auth model, one CLI, structured JSON output, and typed MCP tools across GA4, GTM, Google Ads, Search Console and BigQuery.

Do not build parallel standalone integrations. Extend the patterns already present in gog:

```
Google OAuth / account aliases / keyring / auth setup
                    ↓
official Google API implementation
                    ↓
             typed gog command
                    ↓
        CLI + JSON + typed MCP tool
```

Target UX:

```bash
gog auth setup work@example.com \
  --gcloud-project gog-marketing \
  --create-project \
  --enable-apis \
  --services analytics,tagmanager,googleads,searchconsole,bigquery \
  --open-console

gog auth add work@example.com --services analytics,tagmanager,googleads,searchconsole,bigquery

gog analytics report 123456789 --dimensions=date,country --metrics=sessions
gog analytics properties list
gog tagmanager accounts list
gog googleads customers list
gog googleads query <customer-id> --gaql 'SELECT campaign.id, campaign.name FROM campaign'
gog searchconsole sites list
gog bigquery datasets list --project my-project

gog --account work mcp --list-tools
```

## Hard dependency rule: official Google integrations only

For Google-facing integrations, **do not introduce community/third-party API wrappers or SDKs**.

Use:

| Capability | Required implementation |
|---|---|
| GA4 reporting | Existing `google.golang.org/api/analyticsdata/v1beta` |
| GA4 admin/config | Existing/expanded `google.golang.org/api/analyticsadmin/v1beta` |
| GTM | `google.golang.org/api/tagmanager/v2` |
| Search Console | Existing `google.golang.org/api/searchconsole/v1` |
| BigQuery | Official `cloud.google.com/go/bigquery` |
| Google Ads | **Direct official Google Ads REST API** using gog's authenticated HTTP plumbing / Go stdlib HTTP. No community Go Ads package. |

Google Ads official references:
- REST overview: https://developers.google.com/google-ads/api/rest/overview
- Client library status: https://developers.google.com/google-ads/api/docs/client-libs
- REST auth/headers: https://developers.google.com/google-ads/api/rest/auth
- Version lifecycle: https://developers.google.com/google-ads/api/docs/sunset-dates

The official-only rule applies to **Google API integration dependencies**. Do not gratuitously replace existing upstream gog dependencies such as the existing MCP transport.

---

## Existing code to build on

### GA4 already exists

`internal/cmd/analytics.go` already implements:
- `gog analytics accounts` via Analytics Admin API / `AccountSummaries.List()`
- `gog analytics report` via Analytics Data API / `Properties.RunReport()`
- JSON/TSV output, paging patterns, `--fail-empty`, account resolution.

Use this as the canonical implementation style.

### Search Console already exists

The fork already has `internal/cmd/searchconsole.go` and runtime service wiring for `google.golang.org/api/searchconsole/v1`.

It already exposes sites, Search Analytics/query, sitemaps and URL inspection. **Audit and complete/wire this implementation; do not create a second client.**

### Auth setup already exists

`internal/cmd/auth_setup.go` already supports:
- using/creating a GCP project;
- enabling selected APIs through `gcloud`;
- opening the OAuth client page;
- storing a Desktop OAuth client;
- login;
- multi-account credentials.

Extend this service registry rather than inventing separate setup flows.

### MCP already exists

`internal/cmd/mcp.go` already provides:
- typed fixed-schema MCP tools;
- no generic model-supplied shell runner;
- read-only tools by default;
- write tools hidden unless explicitly enabled;
- service/tool allowlists;
- account pinning;
- JSON subprocess results.

New marketing MCP tools must use this exact security model.

---

# 1. Authentication / Google Cloud bootstrap

## 1.1 Add marketing services to the existing auth service registry

Support:
- `analytics`
- `tagmanager`
- `googleads`
- `searchconsole`
- `bigquery`

Optional convenience alias:
- `marketing` → analytics,tagmanager,googleads,searchconsole,bigquery

Preserve upstream defaults unless a change is required.

## 1.2 API enablement mapping

`gog auth setup --enable-apis` must enable the selected official APIs. Verify current service IDs against official docs / `gcloud services list`, then encode them centrally.

Expected API families:
- Google Analytics Data API
- Google Analytics Admin API
- Google Tag Manager API
- Google Ads API
- Search Console API
- BigQuery API

Do not scatter service IDs across command implementations.

## 1.3 OAuth scopes

Preserve gog's read-only-vs-write scope behaviour.

### GA4
Read: Analytics read-only scope(s).

Write/admin: only the edit/manage scope(s) required by implemented Admin API operations.

Docs:
https://developers.google.com/analytics/devguides/config/admin/v1

### GTM
Read-only:
- `https://www.googleapis.com/auth/tagmanager.readonly`

Write-capable auth: request only scopes required by implemented mutations, including as appropriate:
- `tagmanager.edit.containers`
- `tagmanager.edit.containerversions`
- `tagmanager.publish`
- `tagmanager.manage.accounts`
- `tagmanager.manage.users`
- deletion scope only where delete is implemented.

Scopes:
https://developers.google.com/identity/protocols/oauth2/scopes#tagmanager

### Google Ads
- `https://www.googleapis.com/auth/adwords`

### Search Console
Read:
- `https://www.googleapis.com/auth/webmasters.readonly`

Write:
- `https://www.googleapis.com/auth/webmasters`

### BigQuery
- `https://www.googleapis.com/auth/bigquery`

BigQuery still has IAM/project authorisation on top of OAuth.

## 1.4 One-time GCP bootstrap UX

We cannot remove the Google Cloud project requirement, but gog-marketing should hide it after setup:

1. create/reuse one GCP project;
2. enable requested APIs;
3. guide/open the unavoidable OAuth Desktop-client creation step;
4. store the client once;
5. add multiple user accounts into the existing keyring/account system;
6. all commands reuse that machinery.

No per-service credential files and no separate MCP auth.

---

# 2. GA4: expand existing reporting into Admin/config

Keep existing `analytics accounts` and `analytics report` intact.

Use `google.golang.org/api/analyticsadmin/v1beta`. Prefer stable/beta methods; do not make alpha-only resources core dependencies unless isolated/documented.

## Required read commands

Implement a typed tree along these lines:

```
gog analytics properties list|get
gog analytics datastreams list|get
gog analytics keyevents list|get
gog analytics custom-dimensions list|get
gog analytics custom-metrics list|get
gog analytics audiences list|get
gog analytics googleads-links list|get
gog analytics bigquery-links list|get
gog analytics access-bindings list|get
```

Where the API supports additional high-value config reads cleanly (data retention, reporting settings, change history, enhanced measurement, etc.), include them if stable/beta and consistent with the same resource pattern.

## Required write/config commands

For resources where Google exposes stable/beta mutations, add typed create/update/archive/delete operations, prioritising:
- properties where supported;
- data streams;
- key events;
- custom dimensions;
- custom metrics;
- audiences where supported;
- Google Ads links;
- BigQuery links;
- access bindings/user permissions.

Requirements:
- honour `--readonly`;
- support `--dry-run` where gog's safety model supports it;
- destructive operations use existing confirmation/`--force` conventions;
- JSON output returns the created/updated resource.

Do not substitute a generic untyped JSON passthrough for normal commands.

## Reporting improvements

Do not regress `analytics report`.

Add obvious Data API improvements if straightforward:
- multiple date ranges;
- filters;
- order-bys;
- metadata listing;
- realtime report.

These are secondary to Admin/config coverage.

---

# 3. Google Tag Manager

Use only:
`google.golang.org/api/tagmanager/v2`

Docs:
https://developers.google.com/tag-platform/tag-manager/api/v2

Implement typed commands for the core hierarchy:

```
gog tagmanager accounts list|get
gog tagmanager containers list|get|create|update|delete
gog tagmanager workspaces list|get|create|update|delete
gog tagmanager tags list|get|create|update|delete
gog tagmanager triggers list|get|create|update|delete
gog tagmanager variables list|get|create|update|delete
gog tagmanager folders list|get|create|update|delete
gog tagmanager builtins list|create|delete
gog tagmanager clients list|get|create|update|delete
gog tagmanager environments list|get|create|update|delete
gog tagmanager versions list|get|create|delete|publish
gog tagmanager permissions list|get|create|update|delete
```

Also include current official v2 resources such as destinations / Google tag config where supported and useful.

Normalise IDs/resource paths so callers do not need to manually construct full REST names.

## Workspace lifecycle

Support:
- workspace status;
- workspace sync;
- create version from workspace;
- inspect/list versions;
- publish selected version.

Publishing and deletion are writes/destructive actions and must follow existing safety conventions.

## Rich resource input

For nested GTM objects:
- ergonomic flags for common fields;
- `--json-file` or equivalent for the complete official resource body;
- validate JSON before sending;
- do not invent a divergent unofficial schema.

---

# 4. Google Ads

## Implementation

**No third-party Go Ads SDK.**

Use official REST directly:
https://developers.google.com/google-ads/api/rest/overview

Use gog's OAuth/account machinery to create authenticated HTTP requests.

Centralise the API major version in one package/constant. Current published major at ticket creation is v25; Google's lifecycle is fast-moving:
https://developers.google.com/google-ads/api/docs/sunset-dates

Do not spread version literals throughout commands.

## Ads-specific credentials/headers

Official docs:
https://developers.google.com/google-ads/api/rest/auth

Handle:
- OAuth bearer token from selected gog account;
- `developer-token`;
- optional `login-customer-id` for manager access;
- response `request-id` in useful errors/debug output.

Provide secure developer-token configuration:
- OS keyring/secret store preferred;
- environment variable override acceptable;
- never recommend repo/plaintext secret storage.

Provide flags/config for:
- target customer ID;
- login customer ID / manager ID.

Accept hyphenated customer IDs and normalise to digits for requests.

## Required v1 commands

At minimum:

```
gog googleads customers list
gog googleads query <customer-id> --gaql '...'
gog googleads query <customer-id> --gaql-file query.sql
```

Implement:
- `customers:listAccessibleCustomers`;
- `customers.googleAds:search`;
- pagination;
- structured JSON;
- optional page size/token;
- manager login header;
- clear API errors + request IDs.

If `searchStream` fits cleanly, add `--stream` or a dedicated command.

GAQL is the broad read/reporting primitive for campaigns, ad groups, ads, assets, conversions, metrics, etc.

## Mutations

Do not block this ticket on reproducing the huge mutation surface.

For v1:
- read/reporting via GAQL is mandatory;
- typed, well-tested write operations may be added;
- do not expose a generic arbitrary REST/mutate MCP tool by default.

Document broader mutation coverage as follow-up scope if not implemented here.

---

# 5. Search Console

Treat this as **audit/completion + auth/MCP integration**, not a rewrite.

Official reference:
https://developers.google.com/webmaster-tools/v1/api_reference_index

Verify CLI coverage for:
- sites list/get/add/delete;
- Search Analytics query;
- sitemaps list/get/submit/delete;
- URL Inspection.

References:
- https://developers.google.com/webmaster-tools/v1/searchanalytics/query
- https://developers.google.com/webmaster-tools/v1/urlInspection.index/inspect

Required:
1. verify service registration, scopes and API enablement;
2. fill obvious gaps in inherited CLI;
3. standardise JSON/pagination/errors;
4. expose typed MCP tools;
5. add tests/docs.

Do not add a second Search Console transport.

---

# 6. BigQuery

Use:
`cloud.google.com/go/bigquery`

Docs:
https://docs.cloud.google.com/go/docs/reference/cloud.google.com/go/bigquery/latest

Installed-app/project semantics:
https://docs.cloud.google.com/bigquery/docs/authentication/end-user-installed

## Project semantics

Keep the distinction explicit:
- gog-marketing bootstrap project identifies the installed app;
- BigQuery execution/billing project is the project used by the BigQuery client/query.

Allow project selection via:
1. explicit `--project`;
2. service-specific configured default if cleanly supported;
3. use `--quota-project` only if semantically correct — do not silently conflate it.

Fail clearly if an execution/billing project is required and unresolved.

## Required commands

```
gog bigquery datasets list
gog bigquery datasets get <dataset>

gog bigquery tables list <dataset>
gog bigquery tables get <dataset> <table>
gog bigquery tables schema <dataset> <table>
gog bigquery tables rows <dataset> <table> --max ...

gog bigquery query --project <project> --sql 'SELECT ...'
gog bigquery query --project <project> --file query.sql
gog bigquery query --dry-run ...
```

Requirements:
- Standard SQL by default;
- JSON output with schema + rows;
- bounded row output;
- clear job errors;
- dry-run exposes bytes processed where available;
- context cancellation;
- close clients/resources correctly;
- do not shell out to `bq`.

## Query safety

Arbitrary SQL can mutate data.

Therefore:
- CLI query is available to the operator;
- MCP must **not** classify arbitrary SQL execution as read-only;
- expose dry-run as read-only;
- actual arbitrary SQL execution is write-risk/opt-in under existing MCP permissions unless an official mechanism guarantees read-only execution;
- do not add a home-grown SQL parser merely to guess safety.

---

# 7. MCP exposure

Every useful capability gets a deliberately reviewed typed MCP tool.

Do **not** add:
- `gog_exec`;
- generic argv passthrough;
- generic HTTP request;
- arbitrary model-controlled REST tools.

Follow `internal/cmd/mcp.go`:
- fixed schemas;
- unknown fields rejected;
- service selectors;
- risk classification;
- reads visible by default;
- writes require existing opt-in;
- selected account pinned;
- structured JSON output.

## Minimum read tools

### GA4
- `analytics_accounts`
- `analytics_report`
- `analytics_properties_list/get`
- `analytics_datastreams_list/get`
- list/get tools for implemented Admin resources.

### GTM
- accounts;
- containers list/get;
- workspaces list/get;
- tags list/get;
- triggers list/get;
- variables list/get;
- versions list/get;
- other implemented read resources.

### Google Ads
- `googleads_customers_list`
- `googleads_query`

GAQL maps to one fixed Ads search operation; bound the schema to customer, query, paging and manager-account fields.

### Search Console
- sites list/get;
- query;
- sitemaps list/get;
- inspect.

### BigQuery
- dataset/table/schema/row reads;
- query dry-run.

## Write MCP tools

Expose typed GA4 Admin, GTM and Search Console mutations implemented above as write-risk tools.

GTM publish is an explicit write tool.

BigQuery arbitrary SQL execution is not in the default read set.

Google Ads generic mutation is out of scope unless narrowly typed/reviewed.

Update:
- `gog mcp --list-tools`;
- service selectors for analytics/tagmanager/googleads/searchconsole/bigquery;
- persistent MCP policy docs/examples.

---

# 8. Runtime/service architecture

Follow current dependency injection patterns.

For generated clients:
- add service factories to existing `app.Runtime` / Google service factory;
- compose them in `runtime_services.go`;
- add small helpers like existing analytics/searchconsole service helpers;
- tests inject fake/httptest-backed services.

For Google Ads:
- injected authenticated HTTP client/transport;
- base URL/version injectable in tests;
- centralise headers/error decoding/pagination in a small internal Ads client package;
- do not leak REST boilerplate into commands.

For BigQuery:
- injectable client factory/interface narrow enough for unit tests without a live project.

---

# 9. Output / command conventions

All commands must behave like native gog commands:

- human-readable table/kv output;
- stable `--json`;
- `--plain` where applicable;
- existing `--max`, `--page`, `--all` conventions;
- `--fail-empty` where appropriate;
- `--account` and aliases work identically;
- preserve `--readonly`, `--dry-run`, `--no-input`, `--force`;
- cancellation/timeouts propagate;
- errors expose useful Google details without leaking secrets.

No service invents a different UX.

---

# 10. Documentation

Add/update:
- gog-marketing quickstart;
- auth setup for marketing services;
- GA4 Admin/config;
- GTM commands/workspace lifecycle;
- Google Ads developer-token + manager-account setup;
- Search Console coverage;
- BigQuery project/billing semantics;
- MCP tool table/permissions;
- multi-account examples.

Document explicitly:

> gog-marketing cannot eliminate Google's Cloud project/OAuth requirement. It reduces it to a one-time bootstrap and reuses the same account/keyring/auth layer across services.

Also document the official-only Google dependency policy.

---

# 11. Tests / verification

No live Google credentials required in CI.

## GA4
- existing report/accounts tests stay green;
- Admin read/write paths use injected test services;
- resource-name normalisation;
- paging/empty results.

## GTM
- representative read + mutation operations for each resource family;
- workspace → version → publish;
- destructive confirmation;
- OAuth scope selection.

## Google Ads
Use `httptest` and assert:
- REST URL/version;
- OAuth bearer supplied by gog account client;
- `developer-token`;
- optional `login-customer-id`;
- customer-ID normalisation;
- GAQL body;
- pagination;
- Google Ads error decoding;
- request-ID capture;
- no token leakage.

## Search Console
- inherited tests stay green;
- add gaps for MCP/auth/setup coverage.

## BigQuery
- injectable client factory;
- project resolution;
- datasets/tables/schema/rows;
- query + dry-run shaping;
- arbitrary query MCP risk classification.

## MCP
For every new tool:
- schema rejects unknown/missing/invalid fields;
- correct CLI argv;
- correct read/write risk;
- service selectors work;
- writes absent without write authorisation;
- structured output.

Repo gates at minimum:

```bash
go test ./...
go vet ./...
```

Run existing lint/static-analysis/docs checks defined by CI. Do not weaken gates.

---

# Acceptance criteria

- [x] Existing gog behaviour/upstream-compatible structure preserved.
- [x] No community/third-party Google API SDK introduced.
- [x] Existing GA4 reporting still works.
- [x] GA4 Admin/config has useful typed read + mutation coverage.
- [x] GTM has typed account/container/workspace/tag/trigger/variable/version workflows including publish.
- [x] Google Ads uses direct official REST and supports account listing + GAQL query, manager header, paging and secure developer-token handling.
- [x] Existing Search Console is audited/completed and fully wired into auth/MCP.
- [x] BigQuery uses official Go client and supports dataset/table inspection plus SQL query/dry-run.
- [x] All five services use existing multi-account/alias/keyring model.
- [x] `auth setup --enable-apis --services ...` knows the five marketing services and APIs.
- [x] Read-only auth uses least-privilege read scopes where available.
- [x] CLI commands have stable JSON.
- [x] Typed MCP tools exist for implemented reads and reviewed writes.
- [x] MCP gains no generic shell/HTTP escape hatch.
- [x] Arbitrary BigQuery SQL is not misclassified as read-only MCP.
- [x] Tests require no real Google credentials.
- [x] Setup + multi-account docs complete.
- [x] `go test ./...`, `go vet ./...` and existing CI are green.

## Delivery notes

The v1 core above is implemented. Validation used fakes and `httptest`; no live
Google Cloud project, billing account, BigQuery query, scheduled transfer, or
billable API setup was used.

BigQuery execution is code-enforced with a 1 GiB default `MaximumBytesBilled`
cap and an explicit `--acknowledge-cost`/MCP acknowledgement after dry-run.

The following expansion points remain follow-up work and do not block the v1
acceptance surface:

- GA4 reporting additions such as multiple date ranges, richer filters/order-bys,
  metadata listing and realtime reports.
- Lower-frequency GA4 Admin resources not exposed by the current generated
  `analyticsadmin/v1beta` client used here, notably BigQuery links and access
  bindings, when a current official client surface makes them cleanly typed.
- GTM secondary resources beyond the required account/container/workspace/
  tag/trigger/variable/version workflows, including folders, clients, built-ins,
  environments, destinations and account/container permissions.

---

# Suggested implementation sequence

1. Shared auth/service registry — scopes, API IDs, runtime factories, setup UX.
2. GA4 Admin — easiest extension of existing service; establishes config/write patterns.
3. Search Console — audit inherited implementation and add MCP/auth parity.
4. GTM — official generated client; CRUD + version lifecycle.
5. Google Ads — small internal official-REST client + GAQL read surface.
6. BigQuery — official Go client + project semantics + safe MCP classification.
7. MCP/docs/integration verification.

Prefer existing gog helpers and small internal abstractions over new framework/plumbing.

## Non-goals

- CM360 / DV360 / SA360 (later expansion).
- Rebuilding gog auth/keyring/account management.
- Replacing existing MCP framework.
- Generic arbitrary HTTP/REST MCP.
- Full Google Ads mutation parity in one release.
- Fork-wide rebranding/module-path churn that makes upstream syncing harder.

**Guiding principle: extend gog's proven patterns, use official Google surfaces, and make the marketing stack feel like one product.**
