# OSS scan baseline threat model

## Assets and boundaries

The security gates protect checked-in Go and TypeScript source, workflow
definitions, Dockerfiles, lockfiles, and the locally built release image.
Scanners execute on a trusted self-hosted ARM64 runner. The protected boundary
is the repository checkout; credentials, production services, OAuth accounts,
and package registries are outside this baseline.

## Threats addressed

- Secret leakage: a valid credential or token is checked in.
- Workflow tampering: a PR introduces unsafe GitHub Actions permissions,
  mutable action references, injection, or excessive artifact persistence.
- Unsafe source patterns: dynamic external command construction or direct
  TypeScript code execution.
- Dependency exposure: a high or critical fixed vulnerability reaches a lock
  file or container image.
- Container/config drift: a Dockerfile or image configuration exposes a high
  or critical weakness.

## Threats not covered here

This scanning baseline is not an implementation security audit. It does not
replace tenant isolation, OAuth/MCP authorization, CSRF, prompt-injection,
secret storage, or hosted runner hardening review. Those require a separate
adversarial audit with targeted tests and fixes.

