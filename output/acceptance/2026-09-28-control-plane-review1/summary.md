# Acceptance: frontier-review repair pass

Date: 2026-09-28 Europe/London
Target: PR #11 repair commit for frontier findings F1-F4
Runtime: `http://127.0.0.1:8092`, `memory://` store, loopback-only encrypted file backend

## Expected behavior

Email alone must not mint an admin session. The configured 32-character admin token is required and checked in constant time. The file secret backend must fail closed outside loopback. Actor IDs must survive Postgres restart. Web OAuth must enforce PKCE S256 and OIDC nonce.

## Executed proof

1. Startup with a short admin token failed before binding with `--admin-token must be at least 32 characters`.
2. Login with the correct owner email and wrong admin token remained on `/login` and displayed `This identity is not authorized.`
3. Login with the configured 32-character admin token opened `/connections`.
4. The authenticated page contained `gmail` and `singulyr`, and DOM scanning confirmed the admin token and OAuth client secret were absent.
5. Startup with `--secret-backend file --listen 0.0.0.0:8093` failed with `--secret-backend=file is only allowed with a loopback --listen address`.
6. `TestGoogleOAuthProviderUsesPKCEAndNonce` asserted `code_challenge` and `code_challenge_method=S256` in the authorization URL, verified the exchange sent the bound verifier, and rejected a tampered verifier and mismatched nonce.
7. `TestPostgresStoreMigrationsAndRestart` bootstrapped the owner twice with deliberately different candidate IDs and asserted the persisted IDs were reused, then verified the connection list still contained the OAuth-connected `gmail` record after close/reopen.
8. `TestOwnerAuthenticatorRequiresCredential` verified wrong tokens and unknown emails cannot mint actors.

## Evidence

- Login form: `output/playwright/2026-09-28-control-plane-review1/01-login.png`
- Authenticated desktop list: `output/playwright/2026-09-28-control-plane-review1/02-connections.png`
- Authenticated mobile list: `output/playwright/2026-09-28-control-plane-review1/03-connections-mobile.png`
- Browser console after authenticated flow: clean
- Live Postgres test: PASS
- Fail-closed remote file-backend startup: PASS
- PKCE and nonce tests: PASS

## Result

PASS for F1-F4 repair.

- Wrong-token login denied; correct-token login completed.
- File secrets rejected on non-loopback listeners.
- Persisted user and organisation IDs survived bootstrap restart and retained connections.
- PKCE S256 verifier binding and OIDC nonce mismatch rejection were exercised.

## Remaining risk

Live Google consent and production Secret Manager acceptance remain environment-dependent, as recorded in the primary acceptance summary.
