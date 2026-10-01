# gog-marketing

**Controlled Google marketing access for AI agents and tools.**

![gog-marketing home and access overview](docs/ui/renders/02-home-access-overview.png)

`gog-marketing` gives teams one clear place to connect approved Google
accounts, choose the marketing assets AI tools may use, see whether each
connection is healthy, and review access-configuration changes.

It is an access and permissions layer, not an analytics or reporting
dashboard. It does not display marketing performance data or recommend actions
based on customer data.

## What marketers can see

- Which Google accounts are connected.
- Which Google services are available.
- Which properties, accounts, containers, sites, and projects are enabled.
- Which connections are healthy or need attention.
- What access-configuration changed, when, and who changed it.

Supported services are **Google Analytics, Google Ads, Google Tag Manager,
Search Console, and BigQuery**.

## How it works

1. An administrator connects an approved Google account once.
2. The team chooses the specific marketing assets AI tools may access.
3. Agents use only that approved access, with read-only mode available by
   default.
4. The team can review connection health and access changes at any time.

Google asks for the first login and approval. After that, `gog-marketing`
manages token refresh and routes requests to the approved Google services. No
marketing KPIs, charts, traffic, revenue, ad spend, conversions, impressions,
campaigns, or reports are exposed by this product.

## The revised product UI

The 12 approved interface references are stored in
[`docs/ui/renders`](docs/ui/renders) and tracked for implementation in
[issue #50](https://github.com/Rajeev-SG/gog-marketing/issues/50). They cover the
application shell, Home, service and account cards, asset selection, loading,
attention, partial-failure, onboarding, feedback, and mobile states.

## Getting started

Most marketers only need the product interface. A technical administrator
completes the one-time Google setup below, then the team can connect accounts
and choose assets without handling OAuth files or tokens.

### For your administrator

You need macOS or Linux, Homebrew, a Google Account or Google Workspace
account, and either a `client_secret_*.json` file or permission to create one.
Docker is only needed for the repository acceptance checks.

<details>
<summary>Open the one-time technical setup</summary>

Install the command-line client:

```bash
brew install openclaw/tap/gogcli
gog --version
```

If someone gave you a Google OAuth client file, store it securely:

```bash
gog auth credentials set \
  --client personal-owned \
  ~/Downloads/client_secret_*.json
```

Connect the account and enable only the services the team needs:

```bash
gog auth add you@gmail.com \
  --client personal-owned \
  --services analytics,googleads,tagmanager,searchconsole,bigquery
```

Google opens a browser for the first login and consent. Verify the connection:

```bash
gog auth doctor --check --no-input
```

When no OAuth client file exists, run `gog auth setup you@gmail.com --client
personal-owned` and follow the guided Google Cloud setup. The full walkthrough
is in the [quickstart](docs/quickstart.md).

Keep `client_secret_*.json`, refresh tokens, and Keychain passwords out of
repositories, prompts, logs, screenshots, and agent instructions. If an agent
needs access, give it an approved `gog` command rather than a credential.

</details>

### Safe agent access

Start with approved, read-only, non-interactive commands and explicit account
selection. For example:

```bash
gog --readonly --no-input --json analytics properties list \
  --account you@gmail.com
```

- `--readonly` blocks writes.
- `--no-input` prevents terminal prompts.
- `--json` gives agents stable structured output.
- `--account` makes the selected Google account explicit.

See [Automation](docs/automation.md) and [Safety Profiles](docs/safety-profiles.md)
before granting broader access.

## Connection checks

The repository includes repeatable checks for setup, token health, and
unattended operation:

```bash
make acceptance-local
make acceptance-doctor
make acceptance-live-repeat N=3
```

Routine checks must not open a browser, trigger Keychain dialogs, or wait for
terminal input. See [Unattended Acceptance](docs/acceptance.md).

## Help

| Question | Where to look |
| --- | --- |
| How do I complete Google setup? | [Quickstart](docs/quickstart.md) |
| How do I install `gog`? | [Install](docs/install.md) |
| How should an agent use it? | [Automation](docs/automation.md) |
| How are permissions restricted? | [Safety Profiles](docs/safety-profiles.md) |
| Why does a connection need attention? | [Common problems](#common-problems) |

### Common problems

| What you see | What it means | What to do |
| --- | --- | --- |
| Missing client | No OAuth client is stored | Ask the administrator to run `gog auth credentials set` |
| Unknown account | The requested Google email is not connected | Check `gog auth list --check`, then reconnect the intended account |
| Needs reconnect | Google access is invalid or incomplete | Reconnect that account once |
| Browser opens during a routine check | A token is missing, revoked, or lacks access | Complete the documented one-time bootstrap again |

## Documentation

- [Quickstart](docs/quickstart.md): complete Google Cloud walkthrough.
- [Install](docs/install.md): Homebrew, Docker, Windows, and source builds.
- [Examples](docs/examples.md): common tasks and command patterns.
- [Unattended Acceptance](docs/acceptance.md): the no-browser, no-Keychain contract.
- [Automation](docs/automation.md): JSON output, exit codes, and agent safety.
- [Auth Clients](docs/auth-clients.md): multiple clients and service accounts.
- [MCP](docs/mcp.md): typed agent access without a generic shell bridge.

## Development

The project requires the Go version declared in `go.mod`.

```bash
make build
make test
make ci
```

See [live testing](docs/live-testing.md) for opt-in Google API smoke tests and
[releasing](docs/RELEASING.md) for the maintainer workflow.

<details>
<summary>Full Google service and scope reference</summary>

<!-- auth-services:start -->
| Service | User | APIs | Scopes | Notes |
| --- | --- | --- | --- | --- |
| gmail | yes | Gmail API | `https://www.googleapis.com/auth/gmail.modify`<br>`https://www.googleapis.com/auth/gmail.settings.basic`<br>`https://www.googleapis.com/auth/gmail.settings.sharing` |  |
| calendar | yes | Calendar API | `https://www.googleapis.com/auth/calendar` |  |
| chat | yes | Chat API | `https://www.googleapis.com/auth/chat.spaces`<br>`https://www.googleapis.com/auth/chat.messages`<br>`https://www.googleapis.com/auth/chat.memberships`<br>`https://www.googleapis.com/auth/chat.users.readstate.readonly`<br>`https://www.googleapis.com/auth/chat.messages.reactions.create`<br>`https://www.googleapis.com/auth/chat.messages.reactions.readonly` |  |
| classroom | yes | Classroom API | `https://www.googleapis.com/auth/classroom.courses`<br>`https://www.googleapis.com/auth/classroom.rosters`<br>`https://www.googleapis.com/auth/classroom.coursework.students`<br>`https://www.googleapis.com/auth/classroom.coursework.me`<br>`https://www.googleapis.com/auth/classroom.courseworkmaterials`<br>`https://www.googleapis.com/auth/classroom.announcements`<br>`https://www.googleapis.com/auth/classroom.topics`<br>`https://www.googleapis.com/auth/classroom.guardianlinks.students`<br>`https://www.googleapis.com/auth/classroom.profile.emails`<br>`https://www.googleapis.com/auth/classroom.profile.photos` |  |
| drive | yes | Drive API | `https://www.googleapis.com/auth/drive` |  |
| driveactivity | yes | Drive Activity API | `https://www.googleapis.com/auth/drive.activity.readonly` | Read-only audit/activity scope; authorize with --services driveactivity |
| drivelabels | yes | Drive Labels API | `https://www.googleapis.com/auth/drive.labels.readonly` | Read-only Drive label schema; authorize with --services drivelabels |
| docs | yes | Docs API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/documents` | Export/copy/create via Drive |
| slides | yes | Slides API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/presentations` | Create/edit presentations |
| contacts | yes | People API | `https://www.googleapis.com/auth/contacts`<br>`https://www.googleapis.com/auth/contacts.other.readonly`<br>`https://www.googleapis.com/auth/directory.readonly` | Contacts + other contacts + directory |
| tasks | yes | Tasks API | `https://www.googleapis.com/auth/tasks` |  |
| sheets | yes | Sheets API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/spreadsheets` | Export via Drive |
| people | yes | People API | `profile` | OIDC profile scope |
| forms | yes | Forms API | `https://www.googleapis.com/auth/forms.body`<br>`https://www.googleapis.com/auth/forms.responses.readonly` |  |
| sites | yes | Drive API | `https://www.googleapis.com/auth/drive` | New Google Sites are exposed as Drive files |
| meet | yes | Meet REST API | `https://www.googleapis.com/auth/meetings.space.created`<br>`https://www.googleapis.com/auth/meetings.space.readonly`<br>`https://www.googleapis.com/auth/meetings.space.settings` |  |
| appscript | yes | Apps Script API | `https://www.googleapis.com/auth/script.projects`<br>`https://www.googleapis.com/auth/script.deployments`<br>`https://www.googleapis.com/auth/script.processes` |  |
| analytics | yes | Analytics Admin API, Analytics Data API | `https://www.googleapis.com/auth/analytics.readonly`<br>`https://www.googleapis.com/auth/analytics.edit` | GA4 reporting and typed Admin configuration |
| searchconsole | yes | Search Console API | `https://www.googleapis.com/auth/webmasters` | Search Analytics + sitemap management + URL Inspection |
| tagmanager | yes | Tag Manager API | `https://www.googleapis.com/auth/tagmanager.readonly`<br>`https://www.googleapis.com/auth/tagmanager.edit.containers`<br>`https://www.googleapis.com/auth/tagmanager.edit.containerversions`<br>`https://www.googleapis.com/auth/tagmanager.publish` | GTM accounts, containers, workspaces, resources and version publishing |
| bigquery | yes | BigQuery API | `https://www.googleapis.com/auth/bigquery`<br>`https://www.googleapis.com/auth/bigquery.readonly` | Dataset/table inspection and SQL with explicit execution project |
| adsense | no | AdSense Management API | `https://www.googleapis.com/auth/adsense.readonly` | Consumer OAuth; explicit opt-in with --services adsense; read-only |
| googleads | yes | Google Ads API | `https://www.googleapis.com/auth/adwords` | Official REST access with developer-token and manager-account headers |
| cloudadmin | yes | Cloud Resource Manager API, Service Usage API, Cloud Billing API, BigQuery Data Transfer API, IAM API | `https://www.googleapis.com/auth/cloud-platform` | Narrow cloud administration: inventory, scheduled-transfer disable/delete, API enable/disable |
| groups | no | Cloud Identity API | `https://www.googleapis.com/auth/cloud-identity.groups.readonly` | Workspace only |
| keep | no | Keep API | `https://www.googleapis.com/auth/keep` | Workspace only; service account (domain-wide delegation) |
| admin | no | Admin SDK Directory API | `https://www.googleapis.com/auth/admin.directory.user`<br>`https://www.googleapis.com/auth/admin.directory.group`<br>`https://www.googleapis.com/auth/admin.directory.group.member` | Workspace only; service account with domain-wide delegation required |
| youtube | yes | YouTube Data API v3 | `https://www.googleapis.com/auth/youtube.readonly` | Most read operations also work with API key only (config youtube_api_key or GOG_YOUTUBE_API_KEY) |
| photos | yes | Photos Library API | `https://www.googleapis.com/auth/photoslibrary.readonly.appcreateddata` | Read-only app-created media only after Google Photos Library API scope changes |
| photospicker | no | Photos Picker API | `https://www.googleapis.com/auth/photospicker.mediaitems.readonly` | Consumer OAuth; explicit opt-in with --services photospicker; selected media only |
<!-- auth-services:end -->

</details>
