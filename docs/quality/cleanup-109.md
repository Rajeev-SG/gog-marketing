# Safe codebase quality checks

This page records the #109 cleanup baseline and the quality gates added for
future changes. These gates are evidence-gathering tools, not permission to
delete code automatically. Generated files, tests, migrations, Worker/Durable
Object entry points and external QA harnesses are deliberate parts of the
product surface.

## Commands

```sh
make quality
make quality-metrics
```

`make quality` runs Knip and jscpd with pinned versions. `make
quality-metrics` builds the hosted production client and records its asset
sizes and Vite module count.

## Verified tools

Verified on 2026-10-20:

| Tool | Version | License | Authority checked |
| --- | --- | --- | --- |
| Knip | 6.42.0 | ISC | `npm view knip@6.42.0 version license engines`, publish time older than the existing 24-hour release-age policy, and <https://knip.dev/reference/configuration.html> |
| jscpd | 5.4.1 | MIT | `npm view jscpd version license`, `npm dist-tags.latest`, `jscpd --help`, and <https://github.com/kucherenko/jscpd> |

## Entry-point accounting

### Hosted app

`internal/hosted/app/knip.json` accounts for:

- Cloudflare Worker entry `src/worker.ts`, configured as `main` in
  `wrangler.toml`. It also exports the native `Chat` Durable Object.
- Testable product router `src/index.ts`, imported by the Worker entry.
- Vite HTML entry `chat.html`, which reaches `src/chat/client.tsx`.
- Playwright config, deterministic fixture server, and E2E specs under
  `e2e/`.
- Generated `src/product-css.generated.ts`, imported by server HTML and the
  chat client.
- Generated `src/tool-catalog.generated.json`, imported by
  `src/tool-permissions.ts` and checked by Go runner tests.

### Hosted state

`internal/hosted/state/knip.json` accounts for:

- Production repository entry `src/index.ts`, named as `main` by the
  migration-only Wrangler config.
- Worker compatibility entry `src/worker-runtime.ts`.
- Tests and the Node-only migration loader. SQL migration files are loaded by
  `src/dev-migrations.ts` with `readFileSync`; they are not imported as TS
  modules.

### Tracking Worker

`internal/tracking/worker/knip.json` accounts for `src/index.ts` as the
Wrangler Worker `main`. Its `fetch` and `scheduled` handlers remain the only
runtime entries. `schema.sql` is the D1 schema consumed by Wrangler, not a TS
import.

## Duplication policy

jscpd reports only clone blocks with at least 100 tokens. The low-noise CI
threshold is 1% duplicated lines:

| Scope | Files | Clones | Duplicated lines | Result |
| --- | ---: | ---: | ---: | --- |
| Hosted app production TS/TSX | 26 | 0 | 0 (0.00%) | Pass |
| Hosted state production TS | 6 | 0 | 0 (0.00%) | Pass |
| Tracking Worker production TS | 4 | 0 | 0 (0.00%) | Pass |
| Go production sources | 777 | 63 | 1187 (0.70%) | Pass |

The scans exclude TS tests, generated hosted design/catalog files, Worker
build output, `node_modules`, vendor directories, and Go `_test.go` files.
The 1% threshold is intentionally above the current Go result but below a
value that would hide a new production clone cluster. It does not authorize a
mass refactor.

## Production bundle baseline

Measured with the existing Vite 8 production build before and after this
cleanup:

| Metric | Before | After |
| --- | ---: | ---: |
| Vite modules transformed | 1114 | 1114 |
| `chat.html` | 534 bytes | 534 bytes |
| CSS bundle | 12517 bytes (12.51 kB) | 12517 bytes (12.51 kB) |
| CSS gzip, Vite reporter | 3.08 kB | 3.08 kB |
| JS bundle | 1106195 bytes (1106.19 kB) | 1106195 bytes (1106.19 kB) |
| JS gzip, Vite reporter | 322.99 kB | 322.99 kB |

The hashes were unchanged (`chat-DafiyT4f.css` and
`chat-MSp3Qa0W.js`). No speculative Vite or build-system swap was made.

## Cleanup evidence

The unconfigured hosted-app Knip run reported 30 items: five E2E file hints,
one intentional external QA dependency, and 24 exported-symbol findings.

### Preserved intentional tooling

- `axe-core` stays as a hosted-app dev dependency. It was added for #107 and
  is resolved by the controller's real-source fixture harness with
  `require.resolve("axe-core/axe.min.js")`; no source import is expected for a
  QA CLI/harness dependency. The hosted-app Knip config records this as an
  explicit exception.
- `@playwright/test` is declared by the nested E2E package, not the app
  package. The app config records this cross-package boundary rather than
  duplicating the dependency.
- Tracking Worker package-script tools (`tsc`, `oxfmt`, `oxlint`, and
  `vitest`) are recognized from `package.json` scripts and need no manual
  dependency or binary exceptions.

### Export-surface cleanup

The following 23 hosted-app declarations were used only inside their own
source file. Their unnecessary public `export` markers were removed; the
logic and declarations remain:

- `consentGroupsMarkup`
- `DEFAULT_GOOGLE_ENDPOINTS`
- `CONNECT_ERROR_MESSAGES`
- `auditSafeError`
- `CHAT_GOOGLE_TOOLS`
- `MCP_PROTOCOL_VERSIONS`
- `DEFAULT_MCP_TOOL_CALL_DAILY_LIMIT`
- `DEFAULT_MCP_REQUESTS_PER_MINUTE_LIMIT`
- `jsonResponse`
- `canonicalMcpUrl`
- `PRODUCT_NAVIGATION`
- the `DEFAULT_CHAT_MODEL` re-export in `src/chat/access.ts`
- `ChatSession`
- `DEFAULT_CHAT_MODEL_KEY`
- `MAX_MODEL_INPUT_BYTES`
- `buildOptions`
- `resolveTenant`
- `BearerAuthFailureReason`
- `RunnerServiceState`
- `RunnerServiceStatus`
- `DiscoveredResource`
- `DiscoveryOutcomeStatus`
- `RunnerResponseDiagnostics`

### Type-only deletions

Two type declarations had no source, test, or runtime references:

- `AnalyticsPropertyResult` in
  `internal/hosted/app/src/tool-runner.ts`
- `OpenRecord` in `internal/tracking/worker/src/types.ts`

Both were removed without deleting behavior, tests, or coverage. No runtime
symbol, dependency, file, migration, or generated artifact was deleted.

## CI

`.github/workflows/quality.yml` runs `make quality` only on trusted,
same-repository pull requests. Existing CI, Docker, Pages and release
workflows are unchanged.
