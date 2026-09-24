# `gog analytics custom-metrics`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

GA4 custom metrics

## Usage

```bash
gog analytics (ga) custom-metrics <command>
```

## Parent

- [gog analytics](gog-analytics.md)

## Subcommands

- [gog analytics custom-metrics archive](gog-analytics-custom-metrics-archive.md)
- [gog analytics custom-metrics create](gog-analytics-custom-metrics-create.md)
- [gog analytics custom-metrics get](gog-analytics-custom-metrics-get.md)
- [gog analytics custom-metrics list](gog-analytics-custom-metrics-list.md)
- [gog analytics custom-metrics update](gog-analytics-custom-metrics-update.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--quota-project` | `string` |  | Google Cloud project to bill for API usage (sent as X-Goog-User-Project; some APIs require it with --access-token or ADC) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog analytics](gog-analytics.md)
- [Command index](README.md)
