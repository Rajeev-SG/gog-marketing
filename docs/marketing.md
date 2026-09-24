# Marketing stack

gog-marketing adds typed CLI and MCP access to GA4, Google Tag Manager, Google
Ads, Search Console, and BigQuery while reusing gog's account aliases, keyring,
OAuth client, JSON output, and safety flags.

## Cost and project guardrails

Google Cloud project setup is unavoidable because Google's OAuth client and API
enablement live in a project. Keep that bootstrap project separate from any
execution or billing project.

- Use one small OAuth/bootstrap project for the installed app. Do not attach
  production data or scheduled jobs to it.
- Set the BigQuery execution/billing project explicitly with
  `--billing-project` (the CLI also accepts `--project` after `bigquery`) or
  `GOG_BIGQUERY_PROJECT`. The bootstrap project is never inferred as the billing
  project.
- Run `gog bigquery query --dry-run` before executing SQL. The dry-run reports
  estimated bytes processed and does not execute the query. Every executed
  query requires `--acknowledge-cost` and a `--max-bytes-billed` cap (default
  1 GiB); the MCP tool enforces the same cap and acknowledgement.
- Keep `gog bigquery query` and the `bigquery_query` MCP tool out of read-only
  agent policies. The MCP server classifies arbitrary SQL as write-risk because
  SQL can mutate data and consume billable slots.
- Do not run live Google Cloud setup, BigQuery queries, scheduled transfers, or
  billable validation from CI. Tests use fakes and `httptest` only.
- Use a spend budget/alert on the execution project and inspect the billing
  report after the first real query. A budget alert is not a hard spend cap.

## One-time setup

```bash
gog auth setup work@example.com \
  --gcloud-project gog-marketing \
  --create-project \
  --enable-apis \
  --services marketing \
  --open-console

gog auth add work@example.com --services marketing
```

`marketing` expands to `analytics,tagmanager,googleads,searchconsole,bigquery`.
The setup command enables the official APIs centrally:

| Service | APIs |
| --- | --- |
| analytics | Analytics Admin API, Analytics Data API |
| tagmanager | Tag Manager API |
| googleads | Google Ads API |
| searchconsole | Search Console API |
| bigquery | BigQuery API |

Read-only authorization narrows the scopes to `analytics.readonly`,
`tagmanager.readonly`, `webmasters.readonly`, and `bigquery.readonly`. The
Google Ads OAuth scope remains `adwords`.

## GA4

```bash
gog analytics accounts --all --json
gog analytics report 123456789 --dimensions=date,country --metrics=sessions
gog analytics properties list
gog analytics properties get 123456789
gog analytics datastreams list 123456789
gog analytics keyevents list 123456789
gog analytics custom-dimensions list 123456789
gog analytics custom-metrics list 123456789
gog analytics googleads-links list 123456789
```

Create, update, delete, and archive commands accept a complete official
resource body through `--json-file` (inline JSON, `@file`, or `-`), plus
`--update-mask` where the Admin API supports it. Mutations honour `--readonly`,
`--dry-run`, `--force`, and `--no-input`.

## Google Tag Manager

```bash
gog tagmanager accounts list
gog tagmanager containers list ACCOUNT_ID
gog tagmanager workspaces list ACCOUNT_ID CONTAINER_ID
gog tagmanager tags list ACCOUNT_ID CONTAINER_ID WORKSPACE_ID
gog tagmanager triggers list ACCOUNT_ID CONTAINER_ID WORKSPACE_ID
gog tagmanager variables list ACCOUNT_ID CONTAINER_ID WORKSPACE_ID
gog tagmanager versions list ACCOUNT_ID CONTAINER_ID
```

The typed tree covers account/container/workspace CRUD, tag/trigger/variable
CRUD, workspace status/sync, create-version, version reads/deletion, and guarded
publish. Nested resource mutations accept the official JSON body with
`--json-file`; common name/type/notes flags cover simple cases.

## Google Ads

Google Ads uses the official REST API through gog's authenticated HTTP client.
No third-party Go Ads SDK is used. The API major version is centralized at
`v25` and can be overridden with `--api-version`.

```bash
export GOG_GOOGLE_ADS_DEVELOPER_TOKEN='...'
# Optional manager account for the login-customer-id header:
export GOG_GOOGLE_ADS_LOGIN_CUSTOMER_ID='123-456-7890'

gog googleads customers list --json
gog googleads query 123-456-7890 \
  --gaql 'SELECT campaign.id, campaign.name FROM campaign' \
  --page-size 100 --json
```

The developer token is read from `GOG_GOOGLE_ADS_DEVELOPER_TOKEN` or
`GOOGLE_ADS_DEVELOPER_TOKEN`; keep it in a secret manager or protected
environment, never in a repository. Customer IDs accept hyphens and are
normalized to digits. Errors include the Google request ID when supplied and
never include tokens.

## Search Console

The inherited Search Console implementation remains the single transport:

```bash
gog searchconsole sites list
gog searchconsole query sc-domain:example.com --from 2026-09-01 --to 2026-09-24
gog searchconsole sitemaps list https://example.com/
gog searchconsole inspect https://example.com/ https://example.com/page
```

Sitemap submit/delete follow the same dry-run and destructive-confirmation
rules as other writes.

## BigQuery

BigQuery uses the official `cloud.google.com/go/bigquery` client. The
`--billing-project`/`GOG_BIGQUERY_PROJECT` value is the project charged for
queries and must be explicit.

```bash
gog bigquery datasets list --project my-execution-project
gog bigquery tables list my-dataset --project my-execution-project
gog bigquery tables schema my-dataset events --project my-execution-project
gog bigquery tables rows my-dataset events --max 100 --project my-execution-project

# Never execute before checking the estimate:
gog bigquery query --project my-execution-project --sql 'SELECT 1' --dry-run
gog bigquery query --project my-execution-project --sql 'SELECT 1' \
  --max 100 --max-bytes-billed 1073741824 --acknowledge-cost
```

Standard SQL is the default. Row output is bounded by `--max`; JSON output
contains schema and rows. `--dry-run` is an actual BigQuery dry run and reports
estimated bytes. Arbitrary SQL is exposed to operators but is write-risk in MCP.

## MCP

```bash
gog --account work mcp --allow-tool analytics,tagmanager,googleads,searchconsole,bigquery --list-tools
```

Marketing tools use fixed schemas and the existing service allowlists. Reads
are visible by default. `bigquery_query` and `tagmanager_versions_publish` are
write-risk and require `--allow-write`; `bigquery_query_dry_run` is read-only.
There is no generic shell, HTTP, or arbitrary REST tool.
