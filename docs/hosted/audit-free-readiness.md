# Audit-free hosted readiness — #79 / #52 / #58

## Current milestone (12 October 2026)

This is an **engineering checkpoint, not public-beta readiness or live acceptance**.
The hosted-only consent policy contains 27 eligible service identifiers and an
exact OAuth scope allowlist. Local CLI scopes and capabilities are unchanged.
Google production Data Access classifications and brand/sensitive verification
are **not yet console-verified/approved** for this expanded set. Nothing here
proves the 27 services executable or working through hosted MCP.

The Worker now consumes a generated catalogue of eighty-one native Go
schemas: GA4 property/report reads and data-stream/key-event CRUD; selected-file Drive get, Docs
get/write, Sheets read/update, Slides slide listing/speaker-note replacement;
Calendar event listing/CRUD; eleven Tasks command leaves; GTM version publishing; eight Search Console tools; six Contacts tools; Drive label-schema list/get; Workspace Groups listing/member reads; Meet space create/get/update/end; AdSense account list/get; selected-file Sites metadata/URL reads; five selected Forms tools; Apps Script project get/content/create; partial Chat space/send tools; Classroom course CRUD; and YouTube playlist read/create/delete. All invoke existing typed native handlers. MCP discovery and execution
require a saved account-specific tool grant plus an enabled matching resource
grant when the tool has a resource model. Account-level Tasks tools do not invent assets. Google consent and selecting a resource do not authorize writes
or every future tool. Scope authority comes from encrypted credentials. The native Calendar local-config blocker is now resolved using its explicit-ID
hosted path and selected-calendar timezone; this has fixture evidence only.

Migration `0005_tool_grants.sql` adds compound-FK-owned per-account tool rows.
It preserves only the three previously offered GA4 reads for existing active
accounts with enabled properties; no writes, wildcards, other services or future
tools are seeded. New tools default disabled. The control-plane UI/API saves
explicit tool enablement and separate destructive permission for the selected
account. Unknown/wildcard/billing authority, foreign accounts and partial invalid
batches are rejected. Disconnect cascades tool grants. UI protects account
switches with unsaved changes and prevents stale-loading choices being saved to
another account.

These are **local/PR implementation changes**, not a deployed acceptance claim.
Google Picker selected-file onboarding is now implemented, including account/session
binding and server-side typed Drive verification. Complete 27-service command
coverage and live verification remain required. Workspace bindings cannot access
an arbitrary file: an enabled verified selected-file grant must exist. Real
Picker use remains untested pending operator API-key/project-number configuration.

The runner validates the exact signed tenant/connection/operation/body, explicit
tool allowlist, audit-free and method-appropriate scopes, enabled resource grant
and matching canonical argument before invoking the existing handler. The same
Google transport enforces exact resource/host/method admission before credential
use or network I/O and merges operation cancellation with SDK request
cancellation through response-body consumption. Ordinary CLI transports remain
unchanged. Document and speaker-note replacement additionally require destructive authorization;
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
| `calendar` | `calendar.readonly` | `calendar` | **untested** | Typed discovery/selection, event listing and event CRUD use explicit calendar grants without local aliases; native field-clearing/recurrence plans retained. Fixture CRUD/date/scope/destructive/foreign denials pass; remaining Calendar features and live acceptance pending | [Google](https://developers.google.com/workspace/calendar/api/auth) |
| `chat` | `chat.spaces.readonly chat.memberships.readonly chat.messages.reactions.readonly chat.users.readstate.readonly` | `chat.spaces chat.memberships chat.messages.create chat.messages.reactions.create` | **untested** | Native space list/setup and text send use account/tool grants and approved method scopes; fixture operations/restricted-scope/path denials pass. Restricted history/search/update/delete excluded; member/thread/reaction/attachment adapters and Workspace live acceptance pending | [Google](https://developers.google.com/workspace/chat/authenticate-authorize) |
| `classroom` | `classroom.courses.readonly classroom.rosters.readonly classroom.coursework.students.readonly classroom.coursework.me.readonly classroom.courseworkmaterials.readonly classroom.announcements.readonly classroom.topics.readonly classroom.guardianlinks.students.readonly classroom.profile.emails classroom.profile.photos` | `classroom.courses classroom.rosters classroom.coursework.students classroom.coursework.me classroom.courseworkmaterials classroom.announcements classroom.topics classroom.guardianlinks.students` | **untested** | Native course list/get/create/update/delete use account/tool grants, method scopes, exact course admission and destructive deletion permission; fixture CRUD/denials pass. Other Classroom families/actions, teacher/admin prerequisites and live acceptance pending | [Google](https://developers.google.com/workspace/classroom/guides/auth) |
| `drive` | `drive.file` | `drive.file` | **untested** | `drive_get` guarded read binding implemented; Picker configuration/live acceptance and remaining reads/writes pending | [Google](https://developers.google.com/workspace/drive/api/guides/api-specific-auth) |
| `drivelabels` | `drive.labels.readonly` | No existing service write scope requested | **untested** | Native schema list/get have account/tool grants, exact GET/label-revision admission and drive.labels.readonly checks; fixture reads/denials pass. Selected-file label mutations, Workspace role prerequisites and live acceptance pending | [Google](https://developers.google.com/workspace/drive/labels/guides/authorize) |
| `docs` | `documents.readonly drive.file` | `documents` | **untested** | `docs_get` and `docs_write` guarded bindings implemented; Picker configuration/live acceptance and remaining command coverage pending | [Google](https://developers.google.com/workspace/docs/api/auth) |
| `slides` | `presentations.readonly drive.file` | `presentations` | **untested** | `slides_list_slides` and `slides_update_notes` guarded native bindings implemented; notes replacement requires destructive permission; remaining Slides coverage and live acceptance pending | [Google](https://developers.google.com/workspace/slides/api/scopes) |
| `contacts` | `contacts.readonly contacts.other.readonly directory.readonly` | `contacts` | **untested** | Native list/search/get/create/update/delete use account/tool grants, contact IDs and etag-protected updates; fixture CRUD/field-clearing/denials pass. Batch/dedupe/export/directory/other/file adapters and live acceptance pending | [Google](https://developers.google.com/people/v1/how-tos/authorizing) |
| `tasks` | `tasks.readonly` | `tasks` | **untested** | All eleven native command leaves have fixed hosted tools and account grants; fixture CRUD/completion/recurrence/deletion/partial-failure tests pass. Explicit title/notes clearing is fixed in native serialization and fixture-verified; status reset uses documented needsAction/completed or undo, not an empty status; named-list resolution is not exposed, recurring creation requires count ≤1000. Live acceptance pending | [Google](https://developers.google.com/workspace/tasks/auth) |
| `people` | `profile` | No existing service write scope requested | **untested** | Hosted typed execution adapter pending | [Google](https://developers.google.com/people/v1/how-tos/authorizing) |
| `sheets` | `spreadsheets.readonly drive.file` | `spreadsheets` | **untested** | `sheets_read_range` and `sheets_update_range` guarded bindings implemented; Picker configuration/live acceptance and remaining command coverage pending | [Google](https://developers.google.com/workspace/sheets/api/scopes) |
| `forms` | `forms.body.readonly forms.responses.readonly drive.file` | `forms.body` | **untested** | Native selected-form get/update/publish and response list/get use verified file/tool grants; unpublish requires destructive permission. Picker MIME mapping and fixture persistence/denials pass. Creation/questions/watches and real acceptance pending | [Google](https://developers.google.com/workspace/forms/api/guides/apps-script-setup) |
| `sites` | `drive.file` | No existing service write scope requested | **untested** | Native sites_get/sites_url now use enabled Picker-selected file grants, drive.file and exact GET admission; fixture resource/scope denials pass. Global Drive-backed inventory/search excluded; multi-file URL authorization and live Picker/Sites acceptance pending | [Google](https://developers.google.com/workspace/drive/api/guides/api-specific-auth) |
| `meet` | `meetings.space.readonly` | `meetings.space.created meetings.space.settings` | **untested** | Native space create/get/update/end use account/tool grants, canonical space paths, method scopes and destructive end permission; fixture config persistence/scope denials pass. History/participants, account prerequisites and live acceptance pending | [Google](https://developers.google.com/workspace/meet/api/guides/authenticate-authorize) |
| `appscript` | `script.projects.readonly script.deployments.readonly script.processes` | `script.projects script.deployments` | **untested** | Native project get/content/create use account/tool grants, method scopes and exact project admission; fixture reads/create/denials pass. Function execution stays excluded pending target-scope audit; deployments/versions/bound-file/pull adapters and live acceptance pending | [Google](https://developers.google.com/apps-script/api/how-tos/execute) |
| `analytics` | `analytics.readonly` | `analytics.edit` | **untested** | Native property/report reads and data-stream/key-event CRUD routed with selected-property grants and method scopes; fixture persistence/scope/destructive/file-reference denials pass. Broader Admin operations and live acceptance pending | [Google](https://developers.google.com/analytics/devguides/config/admin/v1/rest) |
| `searchconsole` | `webmasters.readonly` | `webmasters` | **untested** | Eight native site/query/sitemap/inspection tools routed with URL-prefix/domain site grants and method scopes; fixture submission/read-back/deletion and foreign URL/encoding denials pass. Cross-site sitemap authorization, richer query options and live acceptance pending | [Google](https://developers.google.com/webmaster-tools/v1/how-tos/authorizing) |
| `tagmanager` | `tagmanager.readonly` | `tagmanager.edit.containers tagmanager.edit.containerversions tagmanager.publish tagmanager.delete.containers` | **untested** | `tagmanager_versions_publish` uses the existing native handler, canonical enabled-container grants, publish scope and destructive permission; compiler failures are errors. Fixture execution/denial tests pass; live publish/read-back and remaining GTM reads/CRUD pending | [Google](https://developers.google.com/tag-platform/tag-manager/api/v2/authorization) |
| `bigquery` | `bigquery.readonly` | `bigquery` | **untested** | Hosted typed execution adapter pending; connected-project billing safeguards pending | [Google](https://docs.cloud.google.com/bigquery/docs/authentication) |
| `adsense` | `adsense.readonly` | No existing service write scope requested | **untested** | Native account list/get use account/tool grants, adsense.readonly and exact GET/canonical-account admission; fixture reads/scope/path denials pass. Remaining native AdSense resources/reports, publisher prerequisites and live acceptance pending | [Google](https://developers.google.com/adsense/management/reference/rest/v2/accounts/list) |
| `cloudadmin` | `cloud-platform` | `cloud-platform` | **untested** | Hosted typed execution adapter pending; connected-project billing safeguards pending | [Google](https://docs.cloud.google.com/docs/authentication) |
| `groups` | `cloud-identity.groups.readonly` | No existing service write scope requested | **untested** | Both native groups_list/groups_members use account/tool grants, exact read-only scope, bound email lookup and validated membership paths; fixture reads/denials pass. Workspace role/live acceptance pending; no native writes claimed | [Google](https://developers.google.com/identity/protocols/oauth2/scopes) |
| `keep` | `keep` | `keep` | **untested** | Hosted typed execution adapter pending; delegated Workspace authority not yet configured/verified | [Google](https://developers.google.com/workspace/keep/api/reference/rest) |
| `admin` | `admin.directory.user.readonly admin.directory.group.readonly admin.directory.group.member.readonly admin.directory.orgunit.readonly` | `admin.directory.user admin.directory.group admin.directory.group.member admin.directory.orgunit` | **untested** | Hosted typed execution adapter pending; delegated Workspace authority not yet configured/verified | [Google](https://developers.google.com/workspace/admin/directory/v1/guides/authorizing) |
| `youtube` | `youtube.readonly` | `youtube` | **untested** | Native playlist list/create/delete use account/tool grants, mine selector, youtube write scope and destructive deletion permission; fixture persistence/read-scope denials pass. Remaining video/comment/channel/subscription/item features and live acceptance pending | [Google](https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps) |
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

Hosted-app lint, typecheck/build, all 136 app tests, runner tests, and the CLI
and runner binary builds passed at this checkpoint. Three bridge tests exercise
native Sheets write/read handlers over an HTTP fixture, typed-input rejection
and preservation of local MCP. These deterministic scope/callback/refresh/runner
and transport-backed tests are development security evidence only. Live account writes, persistence, second-account isolation,
Picker, reconnect/partial failures, Playwright/accessibility, and the full
Clerk → Google → selection → MCP URL → Codex → Google execution journey remain
**untested for this revision**. No screenshots or real execution logs are claimed.
Do not close #52, #58, #69 or #79 on this checkpoint.

## Local permission UI evidence (11 October 2026)

Playwright exercised rendered native checkbox/button controls using an explicit
**Clerk/API fixture**, not a real login or Google account: enabling and saving a
write for account A, switching to B without inheriting that grant, returning to A
and reading persistence, separate destructive confirmation, disabled Save during
pending account loads, keyboard Space toggling, cancellation of an unsaved-change
account switch, associated labels and a mobile viewport without horizontal
scrolling. Browser console reported zero errors/warnings. This is targeted UI
and accessibility evidence, **not** complete accessibility certification or the
required real Clerk/Google/Codex end-to-end acceptance.

Screenshots and fixture execution logs are under the operator output directory
`/Users/rajeev/output/gog-marketing-parity/`; screenshots are explicitly named
`tool-permissions-*-fixture.png` to avoid confusing them with live acceptance.
Native D1 API tests independently verify persistence, foreign-account rejection
and same-origin mutation safeguards. SQLite/D1 state tests verify migration and
cascade semantics. The generated Worker catalogue has a Go drift regression test
against executable native schemas, so source changes cannot silently advertise
unimplemented or changed tools.

## Picker implementation and external prerequisites

`POST /api/google/picker/start` issues a 5-minute, single-use Picker session bound
to the authenticated tenant, Google connection/subject and exact Clerk session.
Only its SHA-256 hash is stored in migration `0006_picker_sessions.sql`, and it
cascades with the connection. A 50-session/workspace/day cap bounds abuse.
Google's official JS Picker API requires an ephemeral access token in the browser;
this narrow authenticated POST uses `Cache-Control: no-store` and never returns
refresh tokens, OAuth client secrets, signing keys or encryption material.
Do not put the token in URLs, logs, local storage or DOM content. Historical
restricted grants still fail closed before issuance.

The UI uses `PickerBuilder.setDeveloperKey`, `setAppId`, `setOAuthToken`, a Docs
view and the canonical page origin. `POST /api/google/picker/complete` consumes the
bound session and sends only selected IDs plus the authoritative encrypted grant
to a signed private `verify_files` operation. That operation reuses the existing
typed Drive client to fetch exact IDs with `drive.file`, rejecting failed,
trashed, mismatched or shortcut files. Names/MIME types come from Google, not the
browser. Verified grants are saved in one bounded native D1 batch; new files stay
disabled and existing enablement survives. Partial API failures leave successful
files usable and never erase previous choices.

Official references fetched 11 October 2026:
[Picker sample/setup](https://developers.google.com/workspace/drive/picker/guides/sample),
[Picker overview](https://developers.google.com/workspace/drive/picker/guides/overview),
[drive.file policy](https://developers.google.com/workspace/drive/api/guides/api-specific-auth).
The earlier `/guides/enable` link returns 404; the current sample contains setup.
This implementation uses the JavaScript Picker, **not** the separate OAuth
one-pick flow that forbids combined scopes.

**Not provisioned / not verified:** website/API-restricted public
`GOG_GOOGLE_PICKER_API_KEY`, `GOG_GOOGLE_PICKER_APP_ID` (Google project number from
the same project as the central web OAuth client), Picker/Drive API enablement,
production origins and a real account's Google Picker consent. No keys,
subscriptions, resources or paid assessments were created. Missing prerequisites
produce an explicit operator configuration error; no broad Drive fallback exists.

Local tests cover state replay/expiry, sibling sessions/foreign accounts,
spoofed IDs/metadata, encrypted scope authority, ephemeral-only responses, rate
cap, native D1 persistence, default-disabled/new and retained/old grants, and
partial Drive failures. Playwright used a **synthetic Picker API fixture** to
verify captured account binding even after switching accounts mid-selection.
This does not prove real Google Picker, Clerk login or Codex acceptance.

Slides method scope evidence checked 11 October 2026: [presentations.get](https://developers.google.com/workspace/slides/api/reference/rest/v1/presentations/get) permits `drive.file`, `presentations.readonly` or `presentations`; [presentations.batchUpdate](https://developers.google.com/workspace/slides/api/reference/rest/v1/presentations/batchUpdate) permits `drive.file` or `presentations`, not `presentations.readonly`. The hosted notes tool invokes the existing typed handler and is only fixture-verified.

Tasks method scope evidence checked 11 October 2026: [tasklists.list](https://developers.google.com/workspace/tasks/reference/rest/v1/tasklists/list) permits `tasks.readonly` or `tasks`; [tasklists.insert](https://developers.google.com/workspace/tasks/reference/rest/v1/tasklists/insert) requires `tasks`. Native list creation/persisted fixture reads and account-level grant routing do not prove real Google execution.

GTM publish method evidence checked 11 October 2026: [versions.publish](https://developers.google.com/tag-platform/tag-manager/api/reference/rest/v2/accounts.containers.versions/publish) requires `tagmanager.publish`. The existing typed SDK uses POST `tagmanager/v2/{path}:publish`; no Google Ads scope is needed or requested by this handler. Production Console classification and live execution remain unverified.

GA4 Admin method scopes checked 11 October 2026: [dataStreams](https://developers.google.com/analytics/devguides/config/admin/v1/rest/v1beta/properties.dataStreams) and [keyEvents](https://developers.google.com/analytics/devguides/config/admin/v1/rest/v1beta/properties.keyEvents): list/get accept `analytics.readonly` or `analytics.edit`; create/patch/delete require `analytics.edit`. Production Console classification remains unverified.

Search Console method scopes checked 11 October 2026: [sitemap submit](https://developers.google.com/webmaster-tools/v1/sitemaps/submit) and [delete](https://developers.google.com/webmaster-tools/v1/sitemaps/delete) require `webmasters`; [search analytics](https://developers.google.com/webmaster-tools/v1/searchanalytics/query), [sites.list](https://developers.google.com/webmaster-tools/v1/sites/list), [sitemaps.list](https://developers.google.com/webmaster-tools/v1/sitemaps/list) and [inspection](https://developers.google.com/webmaster-tools/v1/urlInspection.index/inspect) accept `webmasters.readonly` or `webmasters`. Actual production classifications/live functionality remain unverified.

Tasks clearing checked 12 October 2026: [Task resource](https://developers.google.com/workspace/tasks/reference/rest/v1/tasks) documents task text fields and status values `needsAction` or `completed`. Explicit empty title/notes now serialize using the existing typed SDK ForceSendFields; omitted fields are unchanged. Payload and persisted fixture read-back tests are not live Google evidence.

Contacts method scopes and paths checked 12 October 2026 against [current official People API discovery](https://people.googleapis.com/$discovery/rest?version=v1) and [People.get](https://developers.google.com/people/api/rest/v1/people/get): connections.list/search/get accept `contacts.readonly` or `contacts`; create/update/delete require `contacts`. SearchContacts results above 30 are capped by Google. Production classification and live execution remain unverified.

Calendar discovery scope evidence checked 12 October 2026: [CalendarList.list](https://developers.google.com/workspace/calendar/api/v3/reference/calendarList/list) supports `calendar.readonly` or `calendar`. Paginated discovery and grant persistence have fixture evidence; a separate native event-execution probe currently fails closed at local alias configuration. Neither is live Google acceptance.

Calendar event blocker resolved 12 October 2026: native explicit-ID listing now executes without local configuration or primary-calendar metadata in hosted mode, using the existing range/listing logic. Local CLI semantics remain unchanged. Successful transport fixture execution is not live Google acceptance.

Calendar event method scopes checked 12 October 2026 in [official Calendar v3 discovery](https://www.googleapis.com/discovery/v1/apis/calendar/v3/rest): event get/instances support `calendar.readonly` or `calendar`; insert/patch/delete support `calendar`. The hosted tools retain that approved scope matrix and do not request secondary Drive/Zoom/Places permissions. Real writes/persistence remain untested.

Drive label schema method scopes checked 12 October 2026 against [official Drive Labels v2 discovery](https://drivelabels.googleapis.com/$discovery/rest?version=v2): list/get permit `drive.labels.readonly`. Hosted code requests no write/admin label scopes or broad Drive scopes for these tools. Google Workspace permissions and real execution remain unverified.

Groups listing checked 12 October 2026 against [official Cloud Identity v1 discovery](https://cloudidentity.googleapis.com/$discovery/rest?version=v1): searchTransitiveGroups supports `cloud-identity.groups.readonly`. Hosted code admits its fixed GET endpoint only and preserves native connected-account query semantics. Actual Workspace privileges and live execution remain unverified.

Groups membership methods checked 12 October 2026 against [official Cloud Identity v1 discovery](https://cloudidentity.googleapis.com/$discovery/rest?version=v1): groups.lookup and memberships.list support `cloud-identity.groups.readonly`. Bound lookup/membership fixture evidence does not prove actual Workspace privileges or Google execution.

Meet method scopes checked 12 October 2026 against [official Meet v2 discovery](https://meet.googleapis.com/$discovery/rest?version=v2): create/endActiveConference require `meetings.space.created`; get accepts created/readonly/settings; patch accepts created/settings. In particular, settings alone cannot end a conference. Production Console classification and live behavior remain unverified.

AdSense account method scopes checked 12 October 2026 in [official AdSense v2 discovery](https://adsense.googleapis.com/$discovery/rest?version=v2): accounts.list/get support `adsense.readonly`. Hosted tools do not request `adwords` or write scopes. Production classification and actual publisher-account execution remain unverified.

Sites metadata checked 12 October 2026 against [official Drive v3 discovery](https://www.googleapis.com/discovery/v1/apis/drive/v3/rest): files.get permits `drive.file`. Existing native Sites get/URL handlers use that endpoint. Broad Drive scopes and global inventory/search are not added; real selected-file execution remains unverified.

Forms method scopes checked 12 October 2026 in [official Forms v1 discovery](https://forms.googleapis.com/$discovery/rest?version=v1): get accepts forms.body/readonly or drive.file; batchUpdate/setPublishSettings accept forms.body or drive.file; response get/list accept forms.responses.readonly or drive.file. Forms Picker consent adds only the approved audit-free drive.file dependency, not broad Drive scopes. Current Google Console classification and real execution remain unverified.

Apps Script project scopes checked 12 October 2026 in [official Script v1 discovery](https://script.googleapis.com/$discovery/rest?version=v1): projects.get/getContent accept `script.projects.readonly` or `script.projects`; create requires `script.projects`. No execution/indirect app scopes are requested or admitted by these tools. Actual API prerequisites and live behavior remain unverified.

Chat methods checked 12 October 2026 in [official Chat v1 discovery](https://chat.googleapis.com/$discovery/rest?version=v1): spaces.list accepts chat.spaces/readonly; native space creation uses spaces.setup with chat.spaces; messages.create permits chat.messages.create. Broad restricted chat.messages/readonly and history endpoints remain excluded. Actual Workspace privileges, production classifications and live behavior remain unverified.

Classroom course method scopes checked 12 October 2026 in [official Classroom v1 discovery](https://classroom.googleapis.com/$discovery/rest?version=v1): list/get support classroom.courses.readonly/courses; create/patch/delete require classroom.courses. Native masks/role constraints are retained. Actual account roles, production classifications and real Google operations remain unverified.

YouTube playlist scopes checked 12 October 2026 in [official YouTube v3 discovery](https://youtube.googleapis.com/$discovery/rest?version=v3): list supports youtube.readonly/youtube; insert/delete support youtube, not youtube.readonly. No upload or unrelated integrations are added. Actual channel prerequisites, production scope classifications and live writes remain unverified.
