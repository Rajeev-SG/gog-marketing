# Acceptance: control-plane connection administration

Date: 2026-09-28 Europe/London
Target: local `gog controlplane` build from branch `gh-9-control-plane`
Runtime: `http://127.0.0.1:8091`, `memory://` control-plane store, encrypted file secret store

## Expected behavior

A configured owner can sign in, see separate `gmail` and `singulyr` connections, open a connection, initiate Google OAuth, review service scopes and status, rename a connection, and see connection/resource administration controls without any refresh token, access token, auth code, or OAuth client secret appearing in the browser response.

## Executed steps

1. Started `go run ./cmd/gog controlplane` with a test OAuth client and local encrypted secret store.
2. Opened `/login` in a fresh Playwright browser and signed in as `owner@example.com`.
3. Verified the Connections list contained `gmail` and `singulyr` with services and `needs_connect` status.
4. Opened `gmail`; verified Connect Google, Reconnect, Validate, rename, discover, and Disconnect were reachable.
5. Activated Connect Google and observed the browser redirect to `accounts.google.com` with the configured `client_id`. Google correctly rejected the non-production `test-client`, proving the hosted OAuth start route and redirect generation.
6. Returned to the connection, renamed it to `gmail-personal`, submitted the CSRF-protected form, and observed the updated detail title and value.
7. Scanned the rendered document for the test client secret and known token strings; none were present.
8. Captured full-page screenshots at desktop and 390x844 mobile viewports and visually inspected both.
9. Started PostgreSQL 17 in Docker and ran `TestPostgresStoreMigrationsAndRestart` with `CONTROL_PLANE_TEST_DATABASE_URL`. The test applied migration 001, rolled it back, reapplied it, persisted an OAuth-connected connection, closed/reopened the store, and verified the record and secret reference survived.

## Evidence

- Browser screenshots: `output/playwright/2026-09-28-control-plane/`
- Login: `01-login.png`
- Desktop connections: `02-connections.png`
- Desktop detail: `03-detail-desktop.png`
- Completed rename mutation: `05-detail-renamed.png`
- Mobile connections: `06-connections-mobile.png`
- Mobile detail: `08-detail-mobile.png`
- Playwright console after authenticated flow: clean
- DOM secret scan: the configured dummy client secret and synthetic token markers absent
- Postgres proof command: `CONTROL_PLANE_TEST_DATABASE_URL=postgres://... go test ./internal/controlplane -run TestPostgresStoreMigrationsAndRestart -count=1`
- Postgres proof result: PASS
- Final reachable actions proven: OAuth start redirect and CSRF-protected rename completed; discover/toggle/disconnect controls reachable in both desktop and mobile layouts.

## Result

PASS for the runnable offline/browser slice and persistence foundation.

- Viewports checked: desktop and 390x844 mobile.
- Sections checked: login, connections list/add form, connection identity/actions, service scopes/rename, approved resources, disconnect.
- Screenshot review: PASS. No overlap, clipping, or incoherent reflow observed.
- Final action completed: connection rename persisted and OAuth start redirected to Google.

## Remaining risk

The issue's live acceptance items for the real `gmail` and `singulyr` Google identities require the production central OAuth client, its registered public callback URL, live Google API access, and one approved resource per connection. Those credentials and deployed callback were not available in this local proof run. Google rejected the intentional `test-client` after the correct consent redirect. Production Secret Manager was implemented behind the same interface but not exercised against a live GCP project. Before closing the production acceptance checklist, deploy with the central client, authorize both identities, discover resources, toggle grants, restart Postgres-backed deployment, and execute one read through the existing engine per connection using an explicitly enabled resource.
