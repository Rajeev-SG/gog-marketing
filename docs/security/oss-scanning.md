# OSS security scanning

This baseline adds local, reproducible OSS gates without paid services,
CodeQL, source uploads, or production changes.

## Coverage and cadence

| Gate | Scanners | Purpose | Cadence |
| --- | --- | --- | --- |
| `make security-fast` | Gitleaks, zizmor | Secret leaks and GitHub Actions policy | Pull request |
| `make security-nightly` | Semgrep, OSV-Scanner, Trivy | Source, dependencies, configuration, and container image | Nightly and manual |

The fast gate intentionally avoids the expensive nightly scanners on every PR.
The nightly gate scans both locally built existing Go images (CLI and hosted runner)
and never publishes them. CLI --help is smoke-tested with networking disabled.

## Commands

```sh
make security-fast
make security-nightly
```

Both commands install their pinned scanner into `.tools/security`, verify the
official release artifact SHA-256 checksum, and write local reports under
`.tools/security/reports/` (outside Git). Workflow gating uses plain output;
other scanners retain SARIF reports. Semgrep runs from a digest-pinned
official image.

## Pinned versions and evidence

Versions, licenses, official Linux/macOS ARM64 artifacts, and checksums were
verified from each project's GitHub release and repository metadata on
2026-10-20:

- Semgrep `1.181.0` — LGPL-2.1; official image Linux ARM64 digest
  `sha256:db0579f992f6597840f96b249119f4999d44ffbc9e3f0a8854c367183e474c24`.
- OSV-Scanner `2.7.0` — Apache-2.0; SHA-256 values are pinned in
  `scripts/security/install-tools.sh`.
- Gitleaks `8.30.1` — MIT; SHA-256 values are pinned in
  `scripts/security/install-tools.sh`.
- Trivy `0.76.0` — Apache-2.0; SHA-256 values are pinned in
  `scripts/security/install-tools.sh`.
- zizmor `1.31.1` — MIT; SHA-256 values are pinned in
  `scripts/security/install-tools.sh`.

The pinned image digest is immutable. Release checksums are compared before a
binary is installed. Upgrades require replacing both the version and checksum.

## Privacy and network controls

- Source remains local. No scanner uploads repository contents or images.
- Gitleaks and zizmor are run locally; zizmor uses `--offline`.
- Semgrep uses the repository's pinned local rules only, and
  `SEMGREP_SEND_METRICS=off` disables metrics collection.
- OSV-Scanner uses native lockfile extraction and offline vulnerability
  databases. It does not submit dependency graphs to the OSV API.
- Trivy downloads vulnerability and policy data only; scanned artifacts remain
  local.
- No third-party scanner action is used beyond the immutable checkout action.

## Findings baseline

The initial scans found accepted findings that are not production source:

- Gitleaks found 17 `generic-api-key` matches in test constants and E2E
  fixtures. Their exact fingerprints are in
  `scripts/security/.gitleaksignore`; new or moved findings still fail.
- zizmor found 12 accepted findings in existing workflows: nine `artipacked`
  medium/low-confidence and three `adhoc-packages` low/high-confidence. They
  are exact file-and-line exceptions in `scripts/security/zizmor.yml`.

The existing workflows use pinned actions but are outside the approved file
scope for this scanning change, so fixing them requires a separate
owner-approved workflow change. Reassess every exception no later than
**2027-01-20**; remove it as soon as the referenced source or workflow is
updated.

The controller is separately landing the known package advisory fix; this
scanner change does not suppress or duplicate that advisory.

The 2026-10-20 nightly run produced these unresolved, unsuppressed results:

- Semgrep: 10 `go.dynamic-external-command` findings in production Go source.
- OSV-Scanner: 50 dependency findings across 23 advisory IDs, spanning Go and
  pnpm lockfiles. The controller's separate package-advisory fix owns remediation.
- Trivy configuration scan: 0 HIGH/CRITICAL findings.
- Trivy locally built image scan: 6 results across 5 advisory IDs, including two
  CRITICAL `pgx` findings and four HIGH findings.

That initial nightly failure was triage input, not accepted risk. Current
post-fix results are recorded below; a reporting-only SARIF exit is never
treated as proof that the workflow gate enforces findings.

## Maintenance

- Nightly scans can change when upstream vulnerability or rule data changes.
- Scanner versions and checksums are reviewed at least monthly.
- Failed findings are triaged by severity, exploitability, and evidence. Do
  not add file-wide audit disables or unexplained suppressions.
- Prefer a precise fix or a scoped exception with evidence and expiry.


## Source-rule triage

The initial constant-only command rule incorrectly treated ten intentionally
configured process paths as vulnerabilities: bootstrap helpers use the stable
signed executable; auth setup selects gcloud by OS; MCP/tenant subprocesses use
the current executable; renderers come from validated local configuration;
browser opening uses a fixed OS map; provider preflight checks its fixed CLI
allowlist before exec. No untrusted executable selection was found at these
callsites. Rather than add file-wide exceptions or refactor safe behavior, the
rule now targets HTTP input reaching process executables or sh/bash command
text. Fixtures require both injected cases to be reported and trusted-config /
non-shell argument cases to remain clean. The direct-TypeScript-execution rule
is unchanged. These are bounded intraprocedural checks, not comprehensive SAST
or proof that all other input sources are safe; the targeted audit and native
boundary tests remain necessary. No production source finding is suppressed.

The module-level OpenPGP warning GO-2026-5932 affects unmaintained
`golang.org/x/crypto/openpgp` packages, not all x/crypto APIs. Production
`go list -deps ./...` at the controller security candidate enumerated773
packages with no OpenPGP import. OSV native Go call analysis is enabled to
make this distinction without an advisory-wide ignore; an introduced reachable
unsafe import must still fail. The nightly job explicitly sets up the pinned
repository Go toolchain for this analysis. Fresh analysis remains required after
dependency changes; the original lockfile-only count is historical, not a
claim of current production exposure.

Controller native Go reachability verification against the patched dependency
candidate exited0, retaining GO-2026-5932 with `called:false` in JSON rather than
ignoring it. No unresolved Go advisory was returned as called. Both current
container builders are included in the final nightly image gate; the original
single-image six-result baseline above remains a historical pre-fix observation.

## Failure-propagating workflow gate

Zizmor SARIF mode intentionally returns0 even for high findings. The PR gate
therefore uses native plain output and its documented nonzero severity exit
codes; it does not depend on paid SARIF/code-scanning services. A deliberately
unsafe, non-runnable fixture shares the exact gate invocation and must produce
high-finding exit14. Tool errors or accidental success fail the regression.
Existing per-location ignores use the actual1-based primary locations, not the
previous off-by-one / step-header lines. No rule or severity class is disabled.

There are13 precise workflow exceptions: the12 legacy findings above and the
prepared quality workflow's pinned Corepack bootstrap at quality.yml:40 (same
known low-severity install pattern as CI, trusted owner/same-repository source,
read-only permissions, no persisted checkout credentials). Existing expiry and
review-on-edit requirements apply. Corrected plain-mode checks, not raw SARIF
success, establish that no unexpected workflow finding remains.

## Verified integrated result — 2026-10-20

Local source contains reviewed UX4e632954 and security-code a94d2895; this does
not mean either PR is merged or production is verified.

| Check | Actual result |
| --- | --- |
| Gitleaks actual source |0 unresolved after17 documented fixture fingerprints |
| Zizmor enforced plain gate |0 unexpected findings after precise known exceptions; high unsafe-workflow fixture returns14 |
| Semgrep |0 production matches; both injected Go cases plus TypeScript fixture detected |
| OSV |0 blocking findings; GO-2026-5932 retained as uncalled, not ignored |
| Trivy configuration |0 HIGH/CRITICAL |
| Trivy CLI image |0 HIGH/CRITICAL; network-disabled --help succeeds |
| Trivy hosted runner image |0 HIGH/CRITICAL; image built locally |

Negative gate probes confirmed Gitleaks returns1 for a generated, unregistered
high-entropy API credential; OSV returns1 for the old vulnerable lockfile in
SARIF mode; Semgrep --error returns1 for unsafe source; zizmor plain returns14
for the deliberately unsafe workflow. The initial alphabetic fake PAT was
recognized as non-secret/sample content, so it was not used as positive proof.
No actual credential, workflow dispatch or image publication was used.

Final full CI and the bounded review of scan introduction remain required.
Native authenticated-browser/Google/provider and production acceptance are
external boundaries, not implied by these source/image checks.
