# Marketer UI verification (3 October 2026)

This is a **development-quality review**, not the real-account deployment gate.
The reviewed starting main was `a8c3c4e7` (PRs #55 and #56). Keep #36 and #5
open until current-commit browser → product → agent/API → live Google acceptance
passes for both owner accounts. Nothing was deployed.

## Browser evidence

The locally compiled server-rendered product handlers were exercised in Chromium
with Playwright and axe, using disposable **synthetic development fixtures**.
Home, account attention, services/tools, asset picker, onboarding, no-results,
duplicate-account feedback, empty-account and long-content layouts were inspected at 1440,
1280, 768 and 390px. These fixtures never replace live-account acceptance.

Local, ignored evidence belongs in `output/playwright/2026-10-03-readiness/`:
 screenshots, axe/overflow results, browser interaction results and review reports.
The final 32 page/viewport checks had zero axe violations, no page-level
horizontal overflow and no JavaScript errors. Keyboard checks exercised skip
navigation and 30 tab stops; real browser cancellation tests covered bulk-save
and unsaved-search confirmations. Development read tests confirmed HTTP 403
is not reported as a successful read, and error feedback allows retry.
Do not commit private account screenshots, IDs, tokens or credential files.

Impeccable critique/polish guided the bounded pass; a separate Vercel Web
Interface Guidelines review found interaction defects. dzhng screenshot
comparison and fresh critique reviewed all twelve approved references against
the implementation, using the explicit adaptations in `DESIGN.md`. The existing
Go architecture, genuine service surface and permissions were retained. No
unsupported navigation, fabricated activity, dashboards or integrations were added.

Fixed defects include insufficient secondary/status text contrast; cramped
mobile account navigation; misleading search-empty recovery; inaccurate submit
feedback; non-success reads being reported as completed; inaccessible read-error
feedback; missing skip navigation; hidden keyboard focus around sticky UI;
and unclear immediate service-wide save/disconnect behaviour. Service actions
now explicitly say they save immediately, include hidden search results, and
warn before discarding unsaved checkbox changes. They do not change grant scope.

The reference review is **not pixel-parity approval**. The approved render map
permits the simpler shell and resource-based service lists. Brand decoration is
not a reason to invent unsupported functionality. Discovery-loading was captured from the actual pending discovery POST with
DevTools native screenshots while transport was held; no simulated loading page
or fabricated progress was used. It shows an honest pending label, not per-service
streaming progress.
Screen-reader announcements and device safe-area behaviour need manual checks;
axe and keyboard tests do not prove those paths.

## Real-account gate and stale tracking

`make acceptance-doctor` finds the encrypted acceptance store and Postgres, but
reports Singulyr as `needs_reconnect`. `make acceptance-live` stops with
`google_session_control` and the deterministic `make acceptance-bootstrap`
remediation. No routine OAuth, Keychain dialog or interactive fallback was used.
Do not repeat terminal authentication failures or count stale manifests as a
current-commit pass. Follow `docs/acceptance.md` for the deliberate human bootstrap,
then rerun live, restart/persistence, denial/isolation and repeat-read acceptance.

Issue #43 remains open: the retained mixed-service proof's Ads failure is
`google_ads_unconfigured`, a configuration short-circuit before any Google Ads
request. Successful GA4/GTM/Search Console reads are genuine historical evidence,
but do not prove #43's required configured-service API/permission failure. Obtain
a safe owner-approved real failure, a successful healthy-service read, and actual
before/after grant snapshots on the current commit. Do not revoke unrelated tokens.

Cloud cleanup remains on hold: the live replacement path and the issue's
backup/transfer/keep safeguards are not yet satisfied. No Cloud deletions or
billable queries were attempted.

## Verification commands

Use the repository's `make build`, `make ci`, `make acceptance-local`,
`make acceptance-doctor`, `make acceptance-live` and, only after successful
bootstrap/live validation, `make acceptance-live-repeat N=3`.

The new encrypted file-keyring path is also covered by the existing real-file
`internal/secrets` tests (provisioning, concurrency, error handling and reopen).
Those are offline regression checks, not live OAuth acceptance. CI repairs here
retain private-file permissions/exclusive creation and preserve wrapped errors;
no authorization or keyring storage model changes were made.
