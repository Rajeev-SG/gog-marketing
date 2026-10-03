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

The owner subsequently connected both real Google accounts through the actual
product in Chrome. Follow-up uses **Browser Relay**, not Ego or a new headless
browser. Private, commit-bound evidence is kept under the operator product-pilot
readiness directory; never publish emails, asset IDs or token material.

The browser → product → agent/API → live Google path passed for GA4, GTM and
Search Console with both accounts; a discovered personal BigQuery dataset also
passed a permission-controlled metadata read. Disabled real assets and
cross-account resources returned 403. Native browser checkbox saves were tested
on both accounts, including filtered saves preserving hidden grants; all original
selections were restored. No query job or billable BigQuery execution was run.

A real seven-scope Google grant exposed an acceptance bug: Google returned
userinfo.email, but the scope checker separately demanded its email alias,
unnecessarily trying to re-consent. Identity aliases are now equivalent; they
grant no service authority. A regression reproduces the actual missing-email
failure, and a negative test retains denied analytics access for identity-only
grants. The existing bootstrap imported fresh owner-approved product tokens
through its token-file inputs; temporary plaintext exports were deleted and
automatic consent fallback was explicitly blocked. Doctor, live acceptance and
three unattended repeat runs pass without further consent or Keychain dialogs.

The real encrypted file-keyring path was also exercised in an isolated private
operator CLI profile: both accounts made three live reads from fresh processes,
with no plaintext refresh tokens found in its stored files and no prompts.

Issue #43 **remains open**. Missing Ads configuration still short-circuits before
a Google request. A read-only BigQuery listing of an explicit existing project
succeeded for both accounts; it did not produce the required real configured
service API/permission failure. Historical and new configuration-unavailable
evidence cannot be substituted for that criterion. Do not revoke working grants,
invent credentials/projects, or disable live APIs to manufacture a failure.

#36 and #5 remain open until all their criteria are met. Ads operator
configuration, a genuine #43 scenario, current cost-boundary/Cloud-cleanup
acceptance and the existing backup/hold requirements are still outstanding.
No Cloud resources were removed or reconfigured; no deployment was performed.

## Verification commands

Use the repository's `make build`, `make ci`, `make acceptance-local`,
`make acceptance-doctor`, `make acceptance-live` and, only after successful
bootstrap/live validation, `make acceptance-live-repeat N=3`.

The new encrypted file-keyring path is also covered by the existing real-file
`internal/secrets` tests (provisioning, concurrency, error handling and reopen).
Those are offline regression checks, not live OAuth acceptance. CI repairs here
retain private-file permissions/exclusive creation and preserve wrapped errors;
no authorization or keyring storage model changes were made.
