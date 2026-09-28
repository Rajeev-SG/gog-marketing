# Unattended Acceptance

Routine development, CI, local acceptance, and live read-only acceptance must be unattended after one explicit bootstrap. They must not use `go run`, open a browser, start OAuth consent, prompt for Keychain access, or wait for terminal input.

## Stable developer profile

Acceptance state lives outside the repository:

```text
~/.config/gog-marketing/acceptance/
  config.json
  master.key
  secrets.json
```

`config.json` is metadata. `master.key` and `secrets.json` are mode 0600 and hold the central OAuth client secret and control-plane refresh-token ciphertext. `GOG_MARKETING_ACCEPTANCE_HOME` overrides the directory; `GOG_MARKETING_ACCEPTANCE_OUTPUT` controls manifests.

## Commands

```bash
make acceptance-local
make acceptance-doctor
make acceptance-live
make acceptance-live-repeat N=3
```

`acceptance-local` provisions a disposable Docker Postgres instance, runs the stable `bin/gog-acceptance` binary with fake OAuth/resources, and cleans up its container. `acceptance-live` uses only the Postgres control plane and the acceptance SecretStore. It never constructs an authorization URL.

## Bootstrap

The only human action is:

```bash
GOOGLE_CLIENT_SECRET_FILE=/path/to/client_secret.json make acceptance-bootstrap
```

This is the single deterministic bootstrap action. It uses the stable signed `gog` binary to export the existing `gmail` and `singulyr` refresh tokens. If either token is genuinely revoked or expired, that same command opens the deliberate browser consent once for the affected account, then exports the replacement token. It imports the central client and tokens into the non-Keychain acceptance SecretStore, silently refreshes both, discovers resources, and enables one resource per connection.

After the central client secret is imported, connect the real `gmail` and `singulyr` accounts through the deliberate control-plane reconnect flow. Once both tokens are in the acceptance SecretStore, routine acceptance is unattended.

## Failure contract

- `google_invalid_grant` / `google_scope_mismatch` → `needs_reconnect`, no browser fallback.
- OAuth client missing → fail fast with `make acceptance-bootstrap`.
- HTTP 429/5xx/network reset → bounded retry only.
- Permission, scope, and disabled-grant errors → immediate failure.

## Why reauthentication happened

Official Google OAuth guidance explains the relevant failure classes:

- [OAuth 2.0 overview — refresh token expiration](https://developers.google.com/identity/protocols/oauth2#refresh-token-expiration)
- [Web-server OAuth flow](https://developers.google.com/identity/protocols/oauth2/web-server)
- [OAuth production readiness](https://developers.google.com/identity/protocols/oauth2/production-readiness/overview)

A refresh token can be invalidated by revocation, six months of non-use, password changes when Gmail scopes are present, exceeding the live refresh-token limit, time-based access, admin restrictions, or GCP session-control policies (`invalid_rapt`). External apps still in **Testing** issue refresh tokens that expire after seven days unless the scopes are limited to basic profile/email. Repeated `prompt=consent` creates additional grants and can cross the per-client live-token limit, silently invalidating the oldest token. The acceptance harness therefore classifies these conditions, never retries terminal auth failures, and does not use forced consent during routine reads.

The Google OAuth publishing/testing status is not exposed through a stable public API or CLI. `acceptance-doctor` reports what it can prove and leaves publishing status as a one-time operator check in the Google Cloud Console.
