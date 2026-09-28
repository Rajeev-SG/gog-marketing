# `gog controlplane`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Web control plane for Google connections and asset grants

## Usage

```bash
gog controlplane (control-plane) [flags]
```

## Parent

- [gog](gog.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--bigquery-projects` | `string` |  | Comma-separated BigQuery projects to discover |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--connection` | `string` |  | Named connection supplying default account/client/quota/billing settings (see gog connection) |
| `--database-url` | `string` |  | Postgres URL; use memory:// only for a disposable local run |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--external-base-url` | `string` |  | Externally reachable base URL used for OAuth callbacks |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--google-ads-developer-token` | `string` |  | Google Ads developer token used for resource discovery |
| `--google-ads-login-customer-id` | `string` |  | Optional Google Ads manager customer ID |
| `--google-client-id` | `string` |  | Central gog-marketing Google OAuth client ID |
| `--google-client-secret` | `string` |  | Central gog-marketing Google OAuth client secret |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--listen` | `string` | 127.0.0.1:8080 | Control-plane listen address |
| `--master-key` | `string` |  | Encryption master key for local secret storage |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--organization-name` | `string` | Default | Initial organisation/workspace name |
| `--organization-slug` | `string` | default | Initial organisation/workspace slug |
| `--owner-email` | `string` |  | Initial owner/admin email allowed to sign in |
| `--owner-name` | `string` | Control-plane owner | Initial owner display name |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--quota-project` | `string` |  | Google Cloud project to bill for API usage (sent as X-Goog-User-Project; some APIs require it with --access-token or ADC) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--secret-backend` | `string` | file | Secret backend: file or secret-manager |
| `--secret-manager-project` | `string` |  | GCP project for Google Secret Manager |
| `--secret-store-path` | `string` |  | Encrypted local secret-store path for file backend |
| `--secure-cookies` | `bool` |  | Mark session cookies Secure (enable behind HTTPS) |
| `--seed-connections` | `string` | gmail,singulyr | Comma-separated named connections to ensure at startup |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--services` | `string` | analytics,tagmanager,googleads,searchconsole,bigquery | Default marketing services for seeded connections |
| `--session-key` | `string` |  | Stable signing key for web sessions |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog](gog.md)
- [Command index](README.md)
