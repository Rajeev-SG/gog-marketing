# Hosted v1 provider inventory

This is the clean-checkout source of truth for hosted v1 provider names. The
Go package `internal/hosted/provider` and `internal/hosted/provider/config.json`
hold the same names for preflight use. This file describes them for operators
and records the historical and conditional commands that provision each
resource.

These are provider **resource names**. Secret values are never committed or
printed. Provisioning commands that prompt for a secret must receive it over
stdin, never as a command-line argument, shell variable expansion in logged
output, or screenshot.

## Reuse, do not recreate

The shared provider resources below are reused when present. Do not create
duplicates of any of them. Resource names in this inventory are the only
authoritative names.

- Cloudflare D1 and KV are the shared Cloudflare resources.
- The Clerk `gog-marketing` application is verified by preflight.
- The Google Cloud project and Artifact Registry are verified by preflight.

The Cloudflare Worker (and its Worker-level bindings) belongs to #64, and the
Cloud Run runner service belongs to #63. Those deployments are planned
upcoming work, not blockers for #59; their parent preflight checks are
currently unavailable.

## Run the safe preflight

Preflight is read-only and reports state without echoing raw provider output,
raw stderr, or secret values:

```sh
make provider-preflight
make provider-preflight PREFLIGHT_FLAGS=--report-only
```

`make provider-preflight` builds the standalone `bin/hosted-preflight` binary
and runs it; it never runs the preflight through `go run`.

`make provider-preflight` exits `0` only when every documented provider check
is present and correct. The `--report-only` variant always exits `0` after
printing the safe JSON report, so it is suitable for collecting evidence when
the hosted build is intentionally incomplete.

Two deadlines bound each run and can be configured through
`PREFLIGHT_FLAGS`:

```sh
make provider-preflight PREFLIGHT_FLAGS="--timeout=10m --command-timeout=5m"
```

`--timeout` (default `5m`) bounds the whole preflight run.
`--command-timeout` (default `90s`) bounds each individual provider CLI
command: one slow command does not consume the deadline budget of the next
commands. Both flags require positive durations. Cold environments — first
credential refresh, wrangler first-run prompts, or slow networks — commonly
exceed shorter defaults, so for first runs on a clean checkout use a larger
`--command-timeout` (for example `5m`). A check that still hits its deadline
is reported `timed-out` with retry advice naming a larger
`--command-timeout`; it is never reported as an access failure.

Preflight verifies, by **name only**:

- provider authentication: `wrangler whoami --json`, `clerk whoami --json`,
  and `gcloud auth list --format=json` (an ACTIVE account is required);
- each documented resource name (D1, KV namespace by exact structured title,
  Clerk application, GCP project, Artifact Registry, Cloud Run service, and
  Cloud Run identity). GCP project, Artifact Registry, and Cloud Run service
  existence are verified from read-only **list-style** scoped queries
  (`gcloud projects list --filter=projectId=<id>`,
  `gcloud artifacts repositories list`,
  `gcloud run services list`) whose output is positively parsed: an empty or
  name-absent structured list proves verified absence, any list entry without
  the expected exact field shape fails closed as `unavailable`, and command
  failures never prove absence;
- each documented non-secret variable name on the Worker (from the latest
  published Worker deployment metadata) and on the Cloud Run runner (from the
  same Cloud Run describe output);
- each documented secret name on the Worker (`wrangler secret list`) and each
  Cloud Run secret-reference name (from the Cloud Run describe output).

It never prints or logs provider output values, raw stderr, or secret values.
A report is `ready` only when every name check verified; an inaccessible check
is reported `unavailable` and keeps the report not-ready. Dependent checks
describe why they were skipped using the parent check's actual state: a parent
reported `missing`, `unavailable`, or `timed-out` produces a matching detail
("not checked because the Cloud Run service is missing/unavailable/timed
out"), and a timed-out parent's details carry retry advice naming a larger
`--command-timeout`. Dependent details never claim a parent is missing unless
the parent was positively verified absent.

Every report starts with an identifier-free `summary` block that groups the
checks for triage. It contains counts only (no resource names, accounts, or
provider output):

```json
{
  "summary": {
    "total": 22,
    "statuses": {"missing": 10, "ok": 8, "unavailable": 4},
    "actions": {"investigate_unavailable": 4, "none": 8, "provision_missing": 10}
  }
}
```

The action categories map directly to remediation paths:
`provision_missing` (create verified-absent resources with the owning
ticket), `investigate_unavailable` (fix provider access, CLI, or output
problems), `fix_config_mismatch` (correct a resource that exists with the
wrong configuration), and `retry_timed_out` (re-run after resolving the
deadline that expired).

Statuses are:

- `ok`: the documented state was positively verified from parsed provider
  output.
- `missing`: verified absence, proven only by positively parsed provider
  responses (an empty or name-absent structured list, a parsed unauthenticated
  whoami payload, or — for the Worker only — the documented Cloudflare error
  code 10007 with the exact sentence `This Worker does not exist on your
  account` emitted by the read-only
  `wrangler deployments status --name <worker> --json` probe, verified with
  wrangler 4.147.0). Raw generic `not found` markers — for example the
  `Worker "name" not found.` text emitted by `wrangler secret list` — never
  prove absence, and there is no Worker list CLI, so any other Worker probe
  failure stays `unavailable`.
- `unavailable`: the check could not be verified. This is the fail-closed
  default for all command failures, including unstructured `not found` text in
  stderr or error strings — raw provider text never proves that a resource is
  absent.
- `mismatch`: the resource exists but has the wrong documented name or
  identity.
- `timed-out`: the total or per-command context deadline expired before the
  provider CLI completed. This is reported separately from `unavailable` so
  operators can distinguish latency from access failures.

Worker existence is verified by the same read-only probe that selects the
active deployment: preflight runs
`wrangler deployments status --name <worker> --json` exactly once, positively
parses the deployment id, requires exactly one version at 100% traffic for the
variable checks, and then reads
`wrangler versions view <version-id> --name <worker> --json` once; only the
binding `name` and `type` fields are interpreted, and binding values are never
emitted. `wrangler secret list --name <worker> --json` is used only for the
documented secret names once the Worker is verified present; it is never an
existence probe. `wrangler versions list` is not used because it reports deployable
versions, including uploads that were never published, so its newest entry
would not prove live deployed readiness. Progressive or traffic-split active
deployments (more than one version, or not exactly one version at 100%) fail
closed as `unavailable` for the Worker variable checks rather than guessing
which version is live.

## Resource inventory

| Provider | Resource | Name | Used for |
| --- | --- | --- | --- |
| Cloudflare | Worker | `gog-marketing` | Hosted control plane and MCP edge (#64) |
| Cloudflare | D1 database | `gog-marketing` | Hosted tenant, connection, grant, and audit state |
| Cloudflare | KV namespace | `gog-marketing` | Cache/ephemeral hosted metadata only |
| Clerk | Application | `gog-marketing` | Hosted identity and MCP authorization server |
| Google Cloud | Project | `gog-marketing-prod` | Hosted data-plane resources |
| Google Cloud | Artifact Registry | `gog-marketing` in `europe-west2` | Cloud Run runner images |
| Google Cloud | Cloud Run service | `gog-marketing-runner` in `europe-west2` | Private Go execution service (#63) |
| Google Cloud | Cloud Run identity | `gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com` | Cloud Run service identity |

The control-plane Worker is the planned #64 resource. D1 remains authoritative
for hosted state; KV is never the source of truth for connections, grants, or
audit history.

## Current observed provider state

On 2026-10-04, the coordinator ran the repaired read-only preflight against
the existing providers. It returned `ready: false` with 22 checks: 10 `missing`, 8 `ok`, 4 `unavailable`.
These are measured results, not inferred deployment readiness.

```json
{
  "wrangler.authentication": "ok",
  "cloudflare.d1_database": "ok",
  "cloudflare.worker": "missing",
  "cloudflare.secret:CLERK_SECRET_KEY": "missing",
  "cloudflare.secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET": "missing",
  "cloudflare.secret:GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY": "missing",
  "cloudflare.secret:GOG_RUNNER_INVOCATION_TOKEN": "missing",
  "cloudflare.env:CLERK_PUBLISHABLE_KEY": "missing",
  "cloudflare.env:CLERK_ISSUER": "missing",
  "cloudflare.env:GOG_GOOGLE_OAUTH_CLIENT_ID": "missing",
  "cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL": "missing",
  "cloudflare.kv_namespace": "ok",
  "clerk.authentication": "ok",
  "clerk.application": "ok",
  "gcloud.authentication": "ok",
  "gcp.project": "ok",
  "gcp.artifact_repository": "ok",
  "gcp.cloud_run_service": "missing",
  "gcp.cloud_run_identity": "unavailable",
  "gcp.cloud_run_env:GOG_GOOGLE_OAUTH_CLIENT_ID": "unavailable",
  "gcp.cloud_run_secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET": "unavailable",
  "gcp.cloud_run_secret:GOG_RUNNER_INVOCATION_TOKEN": "unavailable"
}
```

Worker absence was identified by the scoped deployments probe using the
verified Cloudflare 10007 response. Cloud Run service absence was identified
from a successfully parsed, scoped service list. The Worker binding checks
inherit its verified absence; no secret value or binding value was retrieved.
Cloud Run identity and binding checks remain unavailable because the service
is missing. Provider access or malformed responses are not treated as absence.

The planned deployments belong to #64 (Cloudflare Worker) and #63 (Cloud Run
runner). Their missing state is expected before those tickets and is not a
blocker for shipping the #59 inventory/preflight implementation. No full hosted
browser, Google, or Codex MCP acceptance is claimed by this report.

## Environment and secret names

The table below records **names only**. Do not put values in files, command
arguments, terminal output, screenshots, logs, or pull requests.

| Name | Kind | Surface | Purpose |
| --- | --- | --- | --- |
| `CLERK_PUBLISHABLE_KEY` | non-secret environment | Cloudflare Worker | Public Clerk publishable key identifier |
| `CLERK_ISSUER` | non-secret environment | Cloudflare Worker | Clerk authorization server issuer URL |
| `GOG_GOOGLE_OAUTH_CLIENT_ID` | non-secret environment | Cloudflare Worker, Cloud Run runner | Public Google OAuth client identifier |
| `GOG_CLOUD_RUN_SERVICE_URL` | non-secret environment | Cloudflare Worker | Private Cloud Run runner endpoint |
| `CLERK_SECRET_KEY` | secret | Cloudflare Worker | Clerk server authentication |
| `GOG_GOOGLE_OAUTH_CLIENT_SECRET` | secret | Cloudflare Worker, Cloud Run runner | Google OAuth token exchange |
| `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY` | secret | Cloudflare Worker | Root encryption material for delegated Google credentials |
| `GOG_RUNNER_INVOCATION_TOKEN` | secret | Cloudflare Worker, Cloud Run runner | Shared authentication for the private runner endpoint |

Application behavior tickets may refine binding usage, but they must reuse these
names unless a follow-up issue documents a deliberate rename.

## Provider authentication before preflight

```sh
wrangler whoami --json
clerk whoami --json
gcloud auth list --format=json
```

If any command reports that authentication is missing, complete the normal
interactive provider login separately and do not route credentials through
preflight output.

## Historical and conditional provisioning reference

The commands in this section are **historical or conditional reference only**.
They are reproduced so a clean operator can understand how the provider state
was created and would be recreated from scratch. The D1, KV, Clerk, GCP
project, Artifact Registry, and runner identity commands below already ran and
their resources exist; never run them again or create duplicates. The Worker,
secret, and Cloud Run commands are conditional on #64 and #63. Do not run
provisioning commands from #59; provider mutation remains under coordinator
review.

### Cloudflare Worker, D1, and KV

The Worker is created by the #64 hosted Worker deployment:

```sh
# Conditional: future #64 deployment, from the hosted Worker configuration
# directory. Do not run until #64 owns this step.
wrangler deploy --name gog-marketing
```

The D1 and KV commands below are historical: `gog-marketing` already exists in
each provider.

```sh
# Historical: the D1 database and KV namespace already exist. Do not re-run.
wrangler d1 create gog-marketing
wrangler kv namespace create gog-marketing --binding gog-marketing
```

Once the #64 Worker exists, each documented secret is set by name. The CLI
prompts for the value; do not echo it or use a command-line argument:

```sh
# Conditional: requires the #64 Worker to exist.
wrangler secret put CLERK_SECRET_KEY --name gog-marketing
wrangler secret put GOG_GOOGLE_OAUTH_CLIENT_SECRET --name gog-marketing
wrangler secret put GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY --name gog-marketing
wrangler secret put GOG_RUNNER_INVOCATION_TOKEN --name gog-marketing
```

### Clerk

The Clerk application already exists; the command below is historical only.

```sh
# Historical: the gog-marketing Clerk application already exists.
clerk apps create gog-marketing
```

Then use `clerk link` in the hosted Worker integration checkout when #64 lands,
so the local project is linked to the `gog-marketing` application. Application
IDs may be printed by Clerk; API secret keys must not be.

### Google Cloud project, registry, and runner identity

The project and Artifact Registry already exist; those commands are historical
only. The current runner-identity preflight check is unavailable because the
Cloud Run parent is unavailable.

```sh
# Historical: the project already exists. Do not re-run.
gcloud projects create gog-marketing-prod
# Historical: the registry already exists. Do not re-run.
gcloud artifacts repositories create gog-marketing \
  --repository-format=docker \
  --location=europe-west2 \
  --project=gog-marketing-prod
# Historical: the runner-identity create command was used previously. Do not re-run.
gcloud iam service-accounts create gog-marketing-runner \
  --project=gog-marketing-prod \
  --description="gog-marketing hosted Cloud Run runner identity"
```

The Cloud Run service is provisioned by #63 after its image and endpoint exist:

```sh
# Conditional: owned by #63.
gcloud run deploy gog-marketing-runner \
  --region=europe-west2 \
  --project=gog-marketing-prod \
  --service-account=gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com \
  --no-allow-unauthenticated
```
