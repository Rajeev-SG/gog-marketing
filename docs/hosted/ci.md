# Hosted-product Actions runner

All gog-marketing workflows execute on the repository-specific Oracle runner
`oracle-gog-marketing-arm64` on the existing Oracle VPS. Its labels are
`self-hosted`, `Linux`, `ARM64`, `oracle`, and `gog-marketing`. Registration and
service credentials are stored on that host, never in the repository. The service
is `actions.runner.Rajeev-SG-gog-marketing.oracle-gog-marketing-arm64.service`.

The required CI jobs are `test`, `minimum-go`, `postgres`, `worker`,
`hosted-state`, and `hosted-app`. Windows/macOS jobs were retired by the owner's
explicit decision: the hosted product is deployed on Linux. No Linux/product
test, format, lint, minimum-Go, native D1, or PostgreSQL check was removed.

Because this repository is public, GitHub requires approval for **all external
contributors**. Code-running PR jobs additionally require the repository owner
actor and same-repository head. Do not approve untrusted fork workflows on the
persistent runner. ClawSweeper's maintainer-owned pull_request_target dispatch
never checks out or executes PR-head code.

Docs, container builds and metadata dispatch use the same runner. The leased
AWS Crabbox Actions hydration integration is retired rather than pretending an
Oracle checkout hydrates an AWS lease. `.crabbox.yaml` retains optional direct
CLI lease configuration, without an Actions workflow or ephemeral runner. The upstream reusable macOS signing/release workflow is no
longer called; manual releases validate an existing version tag, build Linux
amd64/arm64 archives using the existing Makefile, and publish checksums in this
repository. Cross-platform Apple signing and upstream Homebrew publishing are
not part of the hosted product's CI.

Inspect runner health with the repository Actions runners page or the GitHub
API. On Oracle, inspect the named systemd service and its journal. A green
workflow requires actual execution of all six required jobs on the final SHA;
registration, an online runner, or a queued workflow alone is not acceptance.
