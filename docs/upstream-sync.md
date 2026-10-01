# Upstream sync and fork releases

`gog-marketing` is a fork of `openclaw/gogcli`. Standard `gog` commands stay
available so upstream improvements can continue to flow into this repository;
the fork adds the marketing, connection, product and access-control surface.

## Sync expectations

Keep an `upstream` remote and sync from its default branch deliberately:

```bash
git remote add upstream https://github.com/openclaw/gogcli.git
git fetch upstream
git merge --no-commit upstream/main
```

Resolve conflicts without dropping the fork's command tree, marketing clients,
product routes, policy checks, acceptance harness or documentation. Before
landing the sync, run:

```bash
make ci
./bin/gog schema --json > /tmp/gog-marketing-schema.json
```

Compare the schema's inherited Workspace commands and marketing additions
against the parity regression test. Keep the `CHANGELOG.md` `Unreleased`
section explicit when a sync changes user-visible behavior.

## Release and install path

The supported installation path is a source build from this fork:

```bash
git clone https://github.com/Rajeev-SG/gog-marketing.git
cd gog-marketing
make build
./bin/gog --version
```

Do not use `brew install openclaw/tap/gogcli`, `ghcr.io/openclaw/gogcli`, or
upstream GitHub releases as `gog-marketing` artifacts. Before public release,
publish a fork-owned binary/package with the same command and policy parity
checks; until then, source builds are the release path.

## Parity checklist

- Build the fork and confirm inherited standard commands and marketing commands.
- Run `make acceptance-local` and the relevant live acceptance target.
- Verify the product UI exposes Workspace and Marketing service groups.
- Verify resource-backed services use resource grants and curated read tools use
  explicit service/tool grants without synthetic resources or wildcard access.
- Verify incremental service enablement requests the union of existing and
  added scopes, preserves prior grants, and can reconnect the same account.
