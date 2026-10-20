# Targeted hosted security audit — 2026-10-20 (#106)

Scope: checked-in hosted app/state/Go runner and GitHub metadata dispatch. No
production probing, real credentials, OAuth consent, provider inference, scanning
uploads, or repository/production configuration changes. This is a bounded audit,
not a claim that the service is vulnerability-free or production-accepted.

## Findings and disposition

| Finding | Exploitability / evidence | Disposition |
|---|---|---|
| GHSA-6qxp-vccf-f47h, upstream high | agents0.28.0 resolved MCP client/core/server2.0.0 and SDK1.30.0. The [upstream advisory](https://github.com/modelcontextprotocol/typescript-sdk/security/advisories/GHSA-6qxp-vccf-f47h) affects HTTP OAuth clients holding credentials when connecting to untrusted MCP servers, not SDK servers. Application code has no remote MCP OAuth client configuration; Google tools use an in-process bridge. No reachable credential-forwarding exploit or compromise demonstrated. | Upgrade only agents to0.28.1, resolving client/core/server2.3.1 and SDK1.32.1. Official registry versions and MIT licence checked. No unrelated dependency updates. Native Workerd regression rejects remote management/lifecycle RPCs and private routes, while legitimate model/history access still succeeds. |
| Dispatcher metadata trust, low / unconfirmed downstream risk | `clawsweeper-dispatch.yml` forwards public issue/PR/comment events using scoped GitHub App tokens. It never checks out or executes PR code, uses environment variables plus jq encoding, and supplies only target IDs. The status-comment author check is not a local dispatch admission gate. Downstream command authorization was not established by this repository audit. | Preserve the required dispatcher; do not claim an unauthorized merge or credential exploit. Before expanding its privileges, verify downstream author/cost authorization or add a trusted-sender gate with dispatcher-owner acceptance. Review by2026-11-19. No scan suppression added. |

No additional confirmed high/critical application vulnerability was found in the
boundaries below. That statement is limited to the inspected paths and tests.
Other dependency/scanner findings remain the separate OSS baseline work, not
implicitly accepted or suppressed here. CodeQL remains deferred.

## Trust boundaries and adversarial evidence

| Boundary / assets | Inspected control and meaningful regression evidence |
|---|---|
| Clerk browser/bearer → internal tenant | `auth.ts` validates configured issuer against publishable key, verified claims, exact OAuth audience and server-derived subject. `index.test.ts` and `mcp.test.ts` reject wrong issuer/audience, expired tokens, inactive tenants and forged identity. Browser session IDs remain mandatory. |
| Browser → Worker/Chat DO | `chat/routes.ts` rejects unknown agent/private paths and cross-origin sockets; `chat/agent.ts` refreshes Clerk/tenant/thread authority on frames and delivery. `chat/integration.test.ts` proves CSRF, foreign conversation and inactive tenant rejection; `chat/native.test.ts` proves independent cohort histories/approvals and excludes silently revoked recipients. |
| Google OAuth → encrypted credential | `connect.ts` binds one-use state to tenant and session; PKCE, nonce/audience and verified userinfo identify the Google account. `connect.test.ts` rejects sibling-session/cross-tenant callbacks and restricted refresh scopes. `credential-crypto.test.ts` proves AES-GCM round-trip, tampering/foreign tenant or connection AAD rejection and key rotation. No token plaintext is stored or passed to a model. |
| MCP/model/browser arguments → Google execution | `mcp.ts`, `tool-permissions.ts` and `chat/tools.ts` recheck fixed schemas, exact tools, service scopes and selected resources. OAuth/data choices are not write authority. `tool-permissions.test.ts` rejects wildcards/billing/foreign grants; native tests reject unapproved/forged actions. `chat/tools.ts` durably binds one-use approval to exact arguments; model text and browser results cannot approve or substitute Google results. Tool text is explicitly untrusted in the system prompt; approvals/grants, not prompt wording, enforce authority. |
| Worker → private Go runner | `runner-auth.ts` signs exact serialized request bytes plus tenant/connection/operation, short expiry and fixed issuer/audience. `runner-auth.test.ts`, `tool-runner.test.ts` and Go `internal/hosted/runner/runner_test.go` exercise capability/identity denial and no-forwarding on redirects. Go typed mappings and guarded transports restrict resource/method/scope; uncertain writes are not replayed. |
| Model request → spend / availability | `chat/model-budgets.ts`, `model-fetch.ts` and `models.ts` enforce atomic application/tenant request/token/currency reservations, paid opt-in and verified existing hard provider caps. Defaults permit free only; no paid fallback. D1 concurrency tests and native quota denial prove no provider dispatch after exhaustion, preserving the sent message. Bound100KB conversations,2048 output tokens and six steps remain. |
| SDK inherited methods → remote network/state | Installed agents0.28.1 dispatch permits only decorated callable methods. App registers only its three intended methods. New native test attempts addMcpServer/removeMcpServer/setState/destroy/onRequest over real Workerd WebSocket RPC and MCP/OAuth/internal HTTP paths; all reject with no attacker/provider call, then model settings/history still work. |
| GitHub events → privileged CI | Existing workflow never executes untrusted PR code, pins its token action, scopes permissions, and shell-quotes environment values. Public metadata admission remains the limited follow-up above; no manual dispatcher job was queued. Dedicated OSS scanning workflow/baseline is reviewed separately. |

Trail of Bits agent-security skills were not installed locally; existing native
source/runtime tests were used instead. No third-party skill code was installed.

## Reproduction / verification

```sh
# From the repository root; fixture-only, no live auth/provider calls.
CI=true make ci TOOLS_DIR=/path/to/existing/pinned/.tools
CI=true pnpm -C internal/hosted/app exec vitest run src/chat/native.test.ts -t 'rejects inherited remote MCP'
```

The dependency candidate passed full local CI (371 app tests) before the added
regression. The new native management test passes independently. Final affected
lint/type/build/app checks, bounded Standards+Spec review and exact-head GitHub CI
still gate integration. External live Clerk/Google/MCP/provider and production
acceptance remain separate; no live compromise/rotation conclusion is inferred.

## Subsequent OSS dependency findings / bounded fixes

The local OSV/Trivy baseline exposed pgx5.7.6 critical upstream decoder
advisories GO-2026-4771/4772, the low-severity simple-protocol dollar-quote SQL
advisory GO-2026-5004, x/net0.59.0 HTTP/2 advisories and Go1.27.1 runtime
advisories. Upgrade pgx to5.9.2 (fixes all three), x/net to0.60.0 and the repository
toolchain plus both digest-pinned Go container builders to1.27.3. The Go1.26.0
language minimum remains unchanged; no other module versions changed.

`TestPgxProtocolSecurityMalformedDecode` reproduces old-version panics for a
short FunctionCall and negative Bind/FunctionCall argument lengths, then passes
with the patched module. Positive encoded Bind/FunctionCall round-trips and the
entire local control-plane package also pass. These directly exercise upstream
**server-side** protocol decoders. The app is a PostgreSQL client and does not
expose Backend.Receive to untrusted clients; no remote app-exploitability claim
is made. Production SQL uses fixed parameterized queries, not attacker-authored
dollar-quoted simple-protocol templates. The compatible package patch is still
preferred to retaining an affected dependency.

Authority: official Go vulnerability JSONs at
<https://vuln.go.dev/ID/GO-2026-4771.json>,
<https://vuln.go.dev/ID/GO-2026-4772.json>,
<https://vuln.go.dev/ID/GO-2026-5004.json>; official Go module proxy versions and
checksums; Go stable-release feed and official multi-platform Docker manifest.
The source release endpoint for pgx5.9.0 returned404; the fixed5.9.2 module/tag
is verified by the official proxy (commit0aeabbcf11d859229c1f0b20e710d3596c76bf27).
Final full CI, fresh image/dependency scans and bounded review remain gates.
