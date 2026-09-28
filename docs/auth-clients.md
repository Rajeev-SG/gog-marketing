# OAuth Clients

Use multiple OAuth client credentials (for different Google Cloud projects or brands) without mixing refresh tokens.

## How it works

- Default client name: `default`
- Default credentials file: `$(os.UserConfigDir())/gogcli/credentials.json`
- Named credentials files: `$(os.UserConfigDir())/gogcli/credentials-<client>.json`
- Tokens are stored per client (`token:<client>:<email>`). Default client also writes legacy keys for backwards compatibility.
- Default account is stored per client, with a legacy global fallback for the default client.

## Selecting a client

Use `--client` (or `GOG_CLIENT`) to pick which credentials + token bucket to use:

```
gog --client work auth credentials ~/Downloads/work-client.json
gog --client work auth add you@company.com
gog --client work gmail search "is:unread"
```

When `--client` is not set, `gog` resolves the client in this order:

1) `--client` / `GOG_CLIENT` override
2) `account_clients` map in config
3) `client_domains` map in config
4) Credentials file named after the email domain (e.g. `credentials-example.com.json`)
5) `default`

## Domain auto-map

To auto-select a client for a domain:

```
gog --client work auth credentials ~/Downloads/work.json --domain example.com
```

This writes `client_domains` into `config.json` so any `@example.com` account selects the `work` client.

## Listing stored credentials

```
gog auth credentials list
```

Shows stored credential files plus any configured domain mappings.

## Config example

```
{
  keyring_backend: "auto",
  account_clients: {
    "you@company.com": "work",
  },
  client_domains: {
    "example.com": "work",
  },
}
```

## Migration notes

- Legacy `token:<email>` entries are copied to `token:default:<email>` the first time they are read.
- Legacy `default_account` is still respected for the default client.
- Browser, manual, remote, and account-manager authorization use S256 PKCE.
  Manual state includes a short-lived verifier under the active `gog` config
  directory. Keep the same `GOG_HOME` and `--client` between remote steps.
- Manual or remote authorization started before v0.24.0 cannot be completed
  after upgrading. Run step 1 again to generate a PKCE-bound URL.

## macOS Keychain prompt loop during development

Symptom: macOS repeatedly shows `gog wants to access key "gogcli" in your keychain`, even after choosing **Always Allow**. Each approval appears to last only for one command.

Cause: `go run ./cmd/gog` compiles a fresh temporary executable for every invocation. macOS Keychain access approval is tied to executable identity, so a new build cannot inherit the previous build's **Always Allow** entry. Rebuilding an ad-hoc-signed binary can have the same effect.

Use one stable executable for authenticated work:

```bash
make build
bin/gog auth list --json
bin/gog auth list --json
```

When the dialog appears for that stable binary, enter the login Keychain password and choose **Always Allow** once. Repeated reads from `bin/gog` should then complete without prompting. Do not use `go run` for commands that read or write stored credentials.

If the binary is rebuilt and macOS prompts again, approve the new binary once or switch to the encrypted file keyring:

```bash
bin/gog auth keyring file
export GOG_KEYRING_BACKEND=file
export GOG_KEYRING_PASSWORD_FILE=/secure/path/gog-keyring-password
```

Keep the password file readable only by the account running `gog`. `gog auth keyring file` changes the backend; it does not migrate existing macOS Keychain entries. Export/import stored tokens and re-store OAuth client credentials before relying on the file backend.

This local Keychain dependency is a development compatibility path. The hosted control plane must use managed secret storage instead of customer or operator macOS Keychain state.

## Quota project

This selects request quota/billing attribution; it does not switch the Google
login, grant IAM access, enable an API, or change a project's billing setup.

Some Google APIs reject ADC or direct access tokens when they cannot identify
a quota project, even if the API is enabled in a project you control. Set
`--quota-project <project-id>` or `GOG_QUOTA_PROJECT` to send that project in
the `X-Goog-User-Project` header on authenticated Google API requests:

```bash
GOG_AUTH_MODE=adc gog --readonly --quota-project my-project calendar events
GOG_AUTH_MODE=adc GOG_QUOTA_PROJECT=my-project gog --readonly calendar events
```

The flag takes precedence over `GOG_QUOTA_PROJECT`. When both are unset, gog
adds no quota-project header and existing authentication behavior is unchanged.
`GOOGLE_CLOUD_QUOTA_PROJECT` and an ADC file's `quota_project_id` do not activate
this setting; configure the flag or gog-specific variable explicitly.

The setting applies to stored OAuth, direct-token, ADC, and delegated
service-account clients. A request's existing `X-Goog-User-Project` header takes
precedence. Gmail watch request clients and MCP commands retain the selected
project. API-key-only requests are unchanged.

The target API must be enabled on the project, and the authenticated principal
needs `serviceusage.services.use` there, such as through
`roles/serviceusage.serviceUsageConsumer`. See Google's
[quota-project requirements](https://docs.cloud.google.com/docs/quotas/set-quota-project).
This option does not grant IAM permissions or OAuth scopes, enable APIs, or
configure billing. Credentials still need the command's API scopes; for example,
ADC Calendar credentials need an appropriate Calendar scope independently of the
quota project.

## Workspace service accounts

Workspace Admin, group, org-unit, and Keep automation commonly run through a
service-account key with domain-wide delegation. Store the key for the
Workspace admin identity you want to impersonate:

```
gog auth service-account set admin@example.com --key ~/Downloads/service-account.json
gog auth service-account status admin@example.com
```

Cloud Identity Groups commands use the same Workspace service-account setup.
Include `https://www.googleapis.com/auth/cloud-identity.groups.readonly` in
domain-wide delegation, then run:

```bash
gog --account admin@example.com groups list
gog --account admin@example.com groups members engineering@example.com
gog --account admin@example.com calendar team engineering@example.com
```

Explicit `--access-token` and `GOG_AUTH_MODE=adc` auth remain available for
advanced environments. `groups list` and Groups backups also require
`--account <workspace-email>` because their transitive membership searches
need the Workspace identity; `groups members` can use the active principal
without that flag. `calendar team` shares the same Groups auth boundary.
Stored user OAuth tokens are not used for these Cloud Identity lookups.

Then run Admin SDK commands with that account:

```
gog --account admin@example.com admin users create ada@example.com \
  --first-name Ada \
  --last-name Lovelace \
  --change-password
```

See [Workspace Admin](workspace-admin.md) for user creation, organizational
units, cleanup, and group examples.
