# Audit-free hosted readiness — #79 / #52 / #58

## Current milestone (11 October 2026)

This is an **engineering checkpoint, not public-beta readiness or live acceptance**.
The hosted-only consent policy contains 27 eligible service identifiers and an
exact OAuth scope allowlist. Local CLI scopes and capabilities are unchanged.
Google production Data Access classifications and brand/sensitive verification
are **not yet console-verified/approved** for this expanded set. Nothing here
proves the 27 services executable or working through hosted MCP.

The Worker implementation now advertises and routes three guarded GA4 reads:
`analytics_properties_get`, `analytics_report`, and `analytics_datastreams_list`.
The last two use the encrypted credential’s actual scope set and the signed
native call; caller context hints cannot supply scope or write authority. These
changes are local/PR code, **not a deployed acceptance claim**. The private
runner also executes a signed `tool_call` operation for nine existing typed
handlers: `analytics_properties_get`, `analytics_report`,
`analytics_datastreams_list`, `drive_get`, `docs_get`, `docs_write`,
`sheets_read_range`, `sheets_update_range`, and `calendar_events`.
This is **private-engine implementation**, not hosted MCP completion. Broader Worker
routing, persisted per-tool/write grants, Picker and remaining service adapters
still have to be completed before advertising the additional tools.

The runner validates the exact signed tenant/connection/operation/body, explicit
tool allowlist, audit-free and method-appropriate scopes, enabled resource grant
and matching canonical argument before invoking the existing handler. The same
Google transport enforces exact resource/host/method admission before credential
use or network I/O and merges operation cancellation with SDK request
cancellation through response-body consumption. Ordinary CLI transports remain
unchanged. Document replacement additionally requires destructive authorization;
USER_ENTERED `IMPORTRANGE` currently requires secondary-file admission and is
blocked by the single-file snapshot rather than leaking an unselected source.

Native Sheets write/read fixture execution passes through the signed runner and
existing Go handler, with a persisted fixture value. Deadline, streaming-body,
redirect/foreign-resource/method-override, scope, disabled-write/tool/resource,
and tampered tenant/body tests are deterministic evidence **only**, not real
Google acceptance. Every expanded service remains **untested live**. Per-command
tracing and real read/write evidence remain required.

## Consent and runtime scope boundary

- `POST /api/google/connect`: `services` selects read permissions;
  `writeServices` is an optional subset requesting additional write scopes.
  Write consent does not itself enable a tool, resource or destructive operation.
- Every scope requested or accepted is checked against the exact hosted allowlist
  in shared `internal/hosted/policy/services.json`, imported by the Worker and
  embedded by the Go runner; each service stores its official source.
  Identity scopes are `openid`, `email`, `userinfo.email`; Google's canonical
  `userinfo.profile` alias is also accepted for `profile`.
- Broad Drive (`drive`, `.readonly`, `.metadata*`, `.activity*`), Gmail, `adwords`,
  restricted Chat history scopes and unknown/transitive scopes are rejected.
  Google Ads is excluded even though the general restricted list omits `adwords`.
- Incremental Google consent remains enabled, but combined grants containing
  excluded permissions are rejected **before credential persistence and discovery**.
  Existing restricted tokens are rejected before refresh or execution. The owner
  cohort must explicitly disconnect/revoke historical grants and reconnect; we do
  not silently sanitize a token while retaining its broader actual authority.
- A returned grant, not a union with historical scope metadata, is authoritative.
  Refresh responses without a scope use the prior grant per the existing provider
  contract; unexpected newly returned scopes fail closed.
- Combined-only scopes (`drive.file`, `cloud-platform`, `keep`) do not provide an
  OAuth-level read/write distinction. Runtime write grants and safety guards are
  still required; explicit consent is not sufficient authority.
- Selected-file Drive access requires Google Picker/app-created-file UX and
  server-side resource grants. No all-Drive inventory/search/audits are permitted.
- Apps Script `run` must not be exposed until the target script's transitive scopes
  are verified against this boundary. Admin/Keep require delegated Workspace
  authority; regular consumer OAuth is not a substitute.
- No paid audits, subscriptions, deployment changes or new infrastructure occurred.

## Service-level capability and evidence matrix

Scope abbreviations below have prefix `https://www.googleapis.com/auth/`, except
`profile`. Read columns can include combined-only scopes. Write columns are
additional scopes on explicit opt-in, not promised callable operations.

| Service | Read consent scopes | Additional write consent | Real verification | Reason / engineering gap | Official API source |
| --- | --- | --- | --- | --- | --- |
| `calendar` | `calendar.readonly` | `calendar` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/calendar/api/auth) |
| `chat` | `chat.spaces.readonly chat.memberships.readonly chat.messages.reactions.readonly chat.users.readstate.readonly` | `chat.spaces chat.memberships chat.messages.create chat.messages.reactions.create` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/chat/authenticate-authorize) |
| `classroom` | `classroom.courses.readonly classroom.rosters.readonly classroom.coursework.students.readonly classroom.coursework.me.readonly classroom.courseworkmaterials.readonly classroom.announcements.readonly classroom.topics.readonly classroom.guardianlinks.students.readonly classroom.profile.emails classroom.profile.photos` | `classroom.courses classroom.rosters classroom.coursework.students classroom.coursework.me classroom.courseworkmaterials classroom.announcements classroom.topics classroom.guardianlinks.students` | **untested** | Hosted typed execution adapter pending; account/role prerequisites unverified | [Google](https://developers.google.com/workspace/classroom/guides/auth) |
| `drive` | `drive.file` | `drive.file` | **untested** | Private typed get/write subset implemented where listed above; Worker/per-tool grants, Picker and remaining command coverage pending | [Google](https://developers.google.com/workspace/drive/api/guides/api-specific-auth) |
| `drivelabels` | `drive.labels.readonly` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/drive/labels/guides/authorize) |
| `docs` | `documents.readonly drive.file` | `documents` | **untested** | Private typed get/write subset implemented where listed above; Worker/per-tool grants, Picker and remaining command coverage pending | [Google](https://developers.google.com/workspace/docs/api/auth) |
| `slides` | `presentations.readonly drive.file` | `presentations` | **untested** | Private typed get/write subset implemented where listed above; Worker/per-tool grants, Picker and remaining command coverage pending | [Google](https://developers.google.com/workspace/slides/api/scopes) |
| `contacts` | `contacts.readonly contacts.other.readonly directory.readonly` | `contacts` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/people/v1/how-tos/authorizing) |
| `tasks` | `tasks.readonly` | `tasks` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/tasks/auth) |
| `people` | `profile` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/people/v1/how-tos/authorizing) |
| `sheets` | `spreadsheets.readonly drive.file` | `spreadsheets` | **untested** | Private typed get/write subset implemented where listed above; Worker/per-tool grants, Picker and remaining command coverage pending | [Google](https://developers.google.com/workspace/sheets/api/scopes) |
| `forms` | `forms.body.readonly forms.responses.readonly` | `forms.body` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/forms/api/guides/apps-script-setup) |
| `sites` | `drive.file` | No existing service write scope requested | **untested** | Private typed get/write subset implemented where listed above; Worker/per-tool grants, Picker and remaining command coverage pending | [Google](https://developers.google.com/workspace/drive/api/guides/api-specific-auth) |
| `meet` | `meetings.space.readonly` | `meetings.space.created meetings.space.settings` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/workspace/meet/api/guides/authenticate-authorize) |
| `appscript` | `script.projects.readonly script.deployments.readonly script.processes` | `script.projects script.deployments` | **untested** | Hosted typed execution adapter pending; execution target scopes must be audited before run | [Google](https://developers.google.com/apps-script/api/how-tos/execute) |
| `analytics` | `analytics.readonly` | `analytics.edit` | **untested** | Guarded MCP get/report/data-stream read routing implemented and fixture-tested; broader Admin writes, per-tool grants and live acceptance pending | [Google](https://developers.google.com/analytics/devguides/config/admin/v1/rest) |
| `searchconsole` | `webmasters.readonly` | `webmasters` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/webmaster-tools/v1/how-tos/authorizing) |
| `tagmanager` | `tagmanager.readonly` | `tagmanager.edit.containers tagmanager.edit.containerversions tagmanager.publish tagmanager.delete.containers` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/tag-platform/tag-manager/api/v2/authorization) |
| `bigquery` | `bigquery.readonly` | `bigquery` | **untested** | Hosted typed execution adapter pending; connected-project billing safeguards pending | [Google](https://docs.cloud.google.com/bigquery/docs/authentication) |
| `adsense` | `adsense.readonly` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/adsense/management/reference/rest/v2/accounts/list) |
| `cloudadmin` | `cloud-platform` | `cloud-platform` | **untested** | Hosted typed execution adapter pending; connected-project billing safeguards pending | [Google](https://docs.cloud.google.com/docs/authentication) |
| `groups` | `cloud-identity.groups.readonly` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending; account/role prerequisites unverified | [Google](https://developers.google.com/identity/protocols/oauth2/scopes) |
| `keep` | `keep` | `keep` | **untested** | Hosted typed execution adapter pending; delegated Workspace authority not yet configured/verified | [Google](https://developers.google.com/workspace/keep/api/reference/rest) |
| `admin` | `admin.directory.user.readonly admin.directory.group.readonly admin.directory.group.member.readonly admin.directory.orgunit.readonly` | `admin.directory.user admin.directory.group admin.directory.group.member admin.directory.orgunit` | **untested** | Hosted typed execution adapter pending; delegated Workspace authority not yet configured/verified | [Google](https://developers.google.com/workspace/admin/directory/v1/guides/authorizing) |
| `youtube` | `youtube.readonly` | `youtube` | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps) |
| `photos` | `photoslibrary.readonly.appcreateddata` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/photos/overview/authorization) |
| `photospicker` | `photospicker.mediaitems.readonly` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/photos/picker/guides/get-started-picker) |

**Excluded:** `gmail`, `googleads`, `driveactivity`; local CLI remains intact.
Tag Manager container deletion uses its separate `tagmanager.delete.containers`
scope, and existing Admin organizational-unit commands require
`admin.directory.orgunit(.readonly)`; these are documented by the linked API
references rather than assumed covered by the initial scope families.

## Evidence and outstanding gates

Official linked API documentation and these policy sources were fetched on
11 October 2026:

- [Restricted scopes](https://support.google.com/cloud/answer/13464325?hl=en)
- [Drive scope classifications / selected-file access](https://developers.google.com/workspace/drive/api/guides/api-specific-auth)
- [Chat sensitive/restricted tables](https://developers.google.com/workspace/chat/authenticate-authorize)
- [Ads-specific restricted classification](https://developers.google.com/google-ads/api/docs/productionize/secure-credentials)
- [GA4 runReport method scopes](https://developers.google.com/analytics/devguides/reporting/data/v1/rest/v1beta/properties/runReport): reporting requires `analytics.readonly` under the approved allowlist; `analytics.edit` alone is not a Data API read grant.
- [Google incremental web-server consent](https://developers.google.com/identity/protocols/oauth2/web-server)

These pages do not replace production Console classification screenshots.
The production scope/verification gate is **blocked pending console evidence**,
not approved based on absence from a general list. Exact per-tool method/role
requirements must be attached during typed execution implementation.

Hosted-app lint, typecheck/build, all 128 app tests, runner tests, and the CLI
and runner binary builds passed at this checkpoint. Three bridge tests exercise
native Sheets write/read handlers over an HTTP fixture, typed-input rejection
and preservation of local MCP. These deterministic scope/callback/refresh/runner
and transport-backed tests are development security evidence only. Live account writes, persistence, second-account isolation,
Picker, reconnect/partial failures, Playwright/accessibility, and the full
Clerk → Google → selection → MCP URL → Codex → Google execution journey remain
**untested for this revision**. No screenshots or real execution logs are claimed.
Do not close #52, #58, #69 or #79 on this checkpoint.
