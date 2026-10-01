# Install

`gog-marketing` builds the same `gog` binary name as upstream and retains the standard `gog` command surface while adding this fork's marketing, connection and product/control-plane capabilities. The visible version is injected at build time: release builds use the tag, while local builds use `git describe`.

## Install gog-marketing

Until this fork has its own packaged release, build **this repository** from
source:

```bash
git clone https://github.com/Rajeev-SG/gog-marketing.git
cd gog-marketing
make build
./bin/gog --version
./bin/gog --help
```

This is the build that contains both the inherited standard `gog` commands and
the fork's marketing/product additions.

### Upstream packages are not this fork

The following installs **upstream `openclaw/gogcli`**:

```bash
brew install openclaw/tap/gogcli
```

Likewise, `ghcr.io/openclaw/gogcli` and releases published from
`openclaw/gogcli` are upstream artifacts. They are useful when you only need
standard `gog`, but they must not be used as acceptance evidence for
`gog-marketing` because they do not necessarily contain this fork's
marketing/product changes.

Fork-owned packaging and release parity are tracked in
[issue #52](https://github.com/Rajeev-SG/gog-marketing/issues/52).

## Headless agents and systemd

For headless agents, configure `gog` with the encrypted file keyring and pass
the same environment to the process that will actually invoke `gog`. A command
working in your login shell only proves that shell has the password; it does
not prove a systemd service, gateway, or agent subprocess inherited it.

Use this as the minimum runtime environment:

```ini
Environment=GOG_KEYRING_BACKEND=file
Environment=GOG_KEYRING_PASSWORD=replace-with-secret-manager-injection
Environment=GOG_HOME=/var/lib/gogcli
Environment=HOME=/home/openclaw
```

Then reload and restart the service before testing from the same entrypoint the
agent uses:

```bash
systemctl --user daemon-reload
systemctl --user restart openclaw-gateway.service

systemctl --user show openclaw-gateway.service \
  --property=Environment

openclaw agent --agent main --message \
  'Run: gog auth doctor --check --no-input && gog gmail search "newer_than:1d" --max 1 --json'
```

If the shell command succeeds but the agent still reports `keyring.password`,
fix the agent or service environment first. Re-authenticating usually does not
help when `gog auth doctor --check` already shows readable tokens in the shell.

## Source builds and platforms

The current supported `gog-marketing` installation path is a source build from
this repository. macOS and Linux can use `make build`. Other platforms can
build the Go command directly with the toolchain declared in `go.mod`.

Do not download an `openclaw/gogcli` release archive and treat it as a
`gog-marketing` release.

Source builds require at least the Go version declared in `go.mod`. The
`toolchain` directive records the preferred toolchain for normal builds and
CI.


## Safety-profile binaries

When `gog` is going to be invoked by an agent, sandbox, or other caller that
should not be able to broaden its own permissions, build a safety-profile
binary instead of the default one. See [Safety Profiles](safety-profiles.md).

```bash
./build-safe.sh safety-profiles/agent-safe.yaml -o bin/gog-agent-safe
./build-safe.sh safety-profiles/readonly.yaml   -o bin/gog-readonly
```

## Verify the install

```bash
gog --version
gog auth keyring         # report current keyring backend
gog --help               # discover top-level commands
```

After running [`gog auth credentials`](commands/gog-auth-credentials.md) and
[`gog auth add`](commands/gog-auth-add.md), `gog auth doctor --check` reports
keyring health, refresh-token validity, and Workspace-specific failure modes.

## Updating

For the fork's current source-build installation:

```bash
git pull
make build
./bin/gog --version
```

Do not use an upstream Homebrew/Docker/release upgrade as though it updates this
fork. Fork-owned packaged upgrades will be documented when #52 lands.

Refresh tokens and OAuth clients remain compatible with ordinary source-build
updates unless a release note explicitly says otherwise.

## Related command pages

- [`gog version`](commands/gog-version.md)
- [`gog auth keyring`](commands/gog-auth-keyring.md)
- [`gog auth credentials`](commands/gog-auth-credentials.md)
- [`gog auth add`](commands/gog-auth-add.md)
- [`gog auth doctor`](commands/gog-auth-doctor.md)
