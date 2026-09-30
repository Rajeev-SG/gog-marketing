# gog-marketing: connect an agent to Google

Use this repository when you want an agent to work with Gmail, Calendar, Drive,
Docs, Sheets, and other Google services without turning Google Cloud into your
day job.

The project contains:

- `gog`, the command-line client your agent calls;
- a control plane that stores connection state and encrypted refresh tokens;
- a one-time bootstrap and an unattended acceptance workflow.

You only deal with Google Cloud once. After setup, the agent talks to `gog`;
Google handles login and consent the first time, and `gog` handles API routing,
scope mapping, and token refresh.

## Start here: connect one Google account

### What you need

- macOS or Linux.
- Homebrew.
- A Google Account or Google Workspace account.
- Either a `client_secret_*.json` file or permission to create one.
- Docker, only if you want to run this repository's acceptance checks.

Do not use `go run` for authenticated commands. Use the installed `gog`
binary.

### 1. Install gog

```bash
brew install openclaw/tap/gogcli
gog --version
```

Other installation methods are in [Install](docs/install.md).

### 2. Store the OAuth client file

If someone gave you a `client_secret_*.json` file, store it under the name this
repository expects:

```bash
gog auth credentials set \
  --client personal-owned \
  ~/Downloads/client_secret_*.json
```

That is the last time you handle the client secret. `gog` copies it into
restrictive per-user storage. Your agent does not parse the file or pass the
secret to Google.

Keep `client_secret_*.json`, refresh tokens, and Keychain passwords out of
repositories, prompts, logs, screenshots, and agent instructions. If the agent
needs a credential, give it the stable `gog` command instead.

If you do not have the file, use the guided setup in
[Google Cloud setup](#google-cloud-setup-only-when-no-client-file-exists).

### 3. Connect the account once

Choose only the services the agent needs:

```bash
gog auth add you@gmail.com \
  --client personal-owned \
  --services gmail,calendar,drive,docs,sheets,contacts
```

Google opens a browser for the first login and consent. That is expected once.
The command stores a refresh token in the operating-system keyring.

A personal `gmail.com` account and most Google Workspace custom domains work
for Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Contacts, and Tasks.

### 4. Verify the connection

```bash
gog auth doctor --check --no-input
```

It must report readable tokens and successful refresh-token exchange. If it
does, the agent can use Google without any further login.

### 5. Give the agent safe read-only commands

Start with read-only, non-interactive, structured output:

```bash
gog --readonly --no-input --json gmail search 'newer_than:7d' --max 5
gog --readonly --no-input --json calendar events --today
gog --readonly --no-input --json drive ls
```

Those flags matter:

- `--readonly` blocks mutating API requests.
- `--no-input` prevents terminal prompts.
- `--json` gives the agent stable structured output.
- `--account you@gmail.com` selects the account explicitly when more than one
  is connected.
- Start with only the services and commands the agent actually needs. Do not
  give it broad write access just because Google supports it.

For stricter agent permissions, see [Automation](docs/automation.md) and
[Safety Profiles](docs/safety-profiles.md).

## Google Cloud setup only when no client file exists

Skip this section if you already stored a `client_secret_*.json` file.

`gog` can guide the one-time setup:

```bash
gog auth setup you@gmail.com \
  --client personal-owned \
  --gcloud-project my-gog-project \
  --enable-apis \
  --open-console
```

It helps with the pieces Google requires once:

1. create or select a Cloud project;
2. enable only the APIs you plan to use;
3. configure the OAuth consent screen;
4. create a Desktop OAuth client;
5. download the `client_secret_*.json` file.

Return to step 2 above after downloading it. The complete manual process is in
the [five-minute quickstart](docs/quickstart.md).

## What the Google Cloud complexity actually means

This table translates the Google vocabulary you will encounter:

| Google Cloud concept | What you do | What the project handles |
| --- | --- | --- |
| Cloud project | Create one only if no client file exists | API and OAuth configuration live here |
| OAuth client | Download and store one JSON file | `gog` stores it securely and uses it automatically |
| Consent screen | Publish the app once for long-lived personal use | The first login still requires your approval |
| OAuth scopes | Name services such as `gmail` or `drive` | `gog` maps service names to the correct scopes |
| Refresh token | Approve the account in a browser once | Stored in Keychain, Secret Service, or Credential Manager |
| Access token | Nothing | Refreshed silently before API calls |
| API endpoints | Nothing | Routed by the `gog` command and typed command schema |
| Control-plane secrets | Nothing after bootstrap | Stored encrypted outside the repository |

The practical rule is simple: Google Cloud is the one-time key cabinet. `gog`
is the tool your agent uses every day.

## What is still a one-time human step

Google requires the first account login and approval. If the OAuth app remains
in Google's **Testing** audience, refresh tokens can expire after seven days.
For long-lived personal use, publish the app once in the same Cloud project.
This changes the app to **In production**; it does not submit it for Google
verification.

Google does not expose that publishing state through a stable public API, so
`gog auth doctor` verifies everything it can and leaves that one Console check
to you.

Workspace-only administration APIs (Admin Directory, Cloud Identity Groups, and
Keep with domain-wide delegation) need a managed Workspace domain and separate
service-account setup. See [Workspace Admin](docs/workspace-admin.md).

## Run this repository's unattended acceptance

This repository proves the connection remains unattended after bootstrap:

```bash
make acceptance-local
make acceptance-doctor
make acceptance-live-repeat N=3
```

These checks use the stable binary, the control plane, and encrypted secret
storage. They must not open a browser, trigger Keychain dialogs, wait for
terminal input, or use authenticated `go run`.

`make acceptance-bootstrap` is the only human action. It imports the central
client and connection tokens. It may open Google consent only when a token is
genuinely missing, revoked, or missing required access. After it succeeds,
`make acceptance-live-repeat N=3` runs unattended. See
[Unattended Acceptance](docs/acceptance.md).

## Common problems

| Symptom | Meaning | Fix |
| --- | --- | --- |
| `missing client` | No OAuth client is stored | Run `gog auth credentials set --client personal-owned <file>` |
| `unknown account` | The requested Google email is not connected | Run `gog auth list --check`, then `gog auth add <email> ...` |
| `needs_reconnect` | The refresh token is invalid or access is incomplete | Reconnect that account once; routine runs must not open OAuth |
| Browser opens during routine acceptance | The token is missing, revoked, or under-scoped | Do not repeat commands; run the documented bootstrap once |
| Weekly Google login returns | OAuth app is still in Testing | Publish the app in Google's Audience page |
| Agent works in Terminal but not its service | The service does not inherit the same credential environment | Configure the service's environment before reauthorizing |

## Commands your agent is likely to need

| Work | Command |
| --- | --- |
| Search mail | `gog gmail search` |
| Read calendar | `gog calendar events` |
| List files | `gog drive ls` |
| Read and write Docs | `gog docs` |
| Read and write Sheets | `gog sheets` |
| Contacts and tasks | `gog contacts`, `gog tasks` |
| Analytics and marketing | `gog analytics`, `gog tagmanager`, `gog googleads`, `gog searchconsole`, `gog bigquery` |

Run `gog --help`, `gog auth services`, or browse the generated
[command index](docs/commands/README.md) for the full surface.

## More documentation

- [Quickstart](docs/quickstart.md): full Google Cloud walkthrough.
- [Install](docs/install.md): Homebrew, Docker, Windows, and source builds.
- [Examples](docs/examples.md): common Gmail, Drive, and Workspace tasks.
- [Unattended Acceptance](docs/acceptance.md): the no-browser, no-Keychain contract.
- [Automation](docs/automation.md): JSON output, exit codes, and agent safety.
- [Auth Clients](docs/auth-clients.md): multiple clients, aliases, and service accounts.
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

`gog` is open source and is not affiliated with Google.

## License

[MIT](LICENSE)

### Local owner pilot: permission-controlled GA4 read

This fork's browser product now has one fixed read operation: Google Analytics property metadata. Sign in, connect Google, select an Analytics property, and save access. **Read property** on the asset picker calls the real product API with that connection's stored token. Disabled or unknown properties are denied before credential retrieval or Google requests. Account grants do not imply access to their properties. Other product API tools are not exposed yet; the older tenant-serving route remains an operator interface.

An authenticated local agent can use `GET /api/connections/<connection-id>/analytics/property?resource=properties/<property-id>` with its private product-session cookie and the session's `X-CSRF-Token` header. Browser calls must be same-origin; cross-site or same-site navigation is rejected before Google calls. This pilot does not yet issue separate agent keys. API responses have four fixed property fields (`name`, `display_name`, `time_zone`, `currency_code`); the normal browser link renders a property page. API responses are JSON; credential-free audit events record allow/deny/error and observed GA4 API request counts (excluding OAuth refresh). A missing session requires sign-in; terminal OAuth failures require reconnect. Discovery reports each Google service separately: successful services remain usable, while unavailable or failing services retain existing selections and show a safe customer-facing status. Do not publish session cookies, resource identities, audit details, or account evidence.

For a Portless named `.localhost` browser origin, use this fork's built `bin/gog controlplane` under Portless and configure `--external-base-url` as the Google-supported `http://localhost:$PORT` loopback origin. The product uses `PORTLESS_URL` to redirect only the loopback OAuth callback back to the named browser origin, where the existing signed-cookie, state, and PKCE checks run. No deployed service is required. Central OAuth setup, Postgres, and encrypted secret-store configuration remain operator responsibilities. After starting or restarting Postgres, wait for `pg_isready` before acceptance. Product and admin cookies have distinct names and can coexist; product sign-out does not clear the admin session.

This is a narrow implementation milestone, **not completed real-account acceptance or deployment approval**. Issue #36's two-account, restart, reconnect, repeated-run, and full service requirements remain open. Synthetic development checks do not satisfy that gate. Do not deploy or publish automatic previews before the complete local real-account gate passes.
