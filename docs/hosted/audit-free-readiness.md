# Audit-free hosted readiness — #79 / #52 / #58

## Native Forms questions candidate (19 October 2026)

Three fixed-schema adapters reuse native `forms questions add/move/delete`
(and map the existing direct aliases): `forms_add_question`,
`forms_move_question` and `forms_delete_question`.
Creation preserves text/paragraph/radio/checkbox/dropdown/scale/date/time,
required answers, ordered choices, quiz answers/points, append/explicit position,
description, scale labels/bounds and date/time flags. Pure native planning
validates the input; no arbitrary item JSON or file/source input is accepted.

Exact enabled form/tool write authority and existing approvals are required.
Deletion additionally requires destructive permission before any Google call.
Only `forms.body` or `drive.file` are admitted. GET selected-form preflight and
POST selected `:batchUpdate` are pinned; moving is POST only. Official Forms
v1 discovery checked 19 October 2026 confirms these scopes. No publishing,
watch mutations, external/data-source operation or new OAuth scope is exposed.

Signed native stateful fixtures decode SDK requests, persist each of the eight
question types and grading, and check append, index zero, move and delete
preservation plus authorization denials. D1/Clerk fixtures verify matching
write/destructive snapshots and foreign/disabled resource/tool isolation.
All three tools remain **unverified live** under #97; no product deployment,
real Google mutation, permission, credential or billing change is claimed.
Candidate totals: **184 / 146 active tools**, **151 mapped native leaves**,
**150 requiring method/scope/resource audit**. #96 preclosed state is untouched.

## Explicit Slides edits candidate (19 October 2026)

- `slides_replace_text` reuses native `slides replace-text`, preserving exactly
  one explicit selection: `all=true`, nonempty `page_ids`, or `object_id`.
  Find/replacement text is literal, including whitespace and Unicode. Empty
  replacement is supported only with the separate destructive permission.
- `slides_skip_slide` and `slides_unskip_slide` reuse native visibility handlers
  on one explicitly selected slide; unskip serializes false explicitly.

All three require exact enabled tool/file write authority and existing approval
controls. Only `presentations` or `drive.file` are admitted, not readonly scopes.
Skip/unskip POST only to the selected presentation's `:batchUpdate`; object
replacement may first GET that same presentation and preserves its revision
check. No implicit presentation-wide selection, raw batch body, source file,
external fetch, new OAuth scope or data-source operation is exposed.

Signed native fixtures decode actual outgoing request kinds, independently
apply UTF-16 deletion/insertion and replacements, and verify scope preservation
plus skip/unskip state. Missing/foreign/disabled grants, readonly scopes and
empty replacement without destructive permission fail before Google. Six
D1/Clerk cases verify write/destructive snapshots and tenant/resource isolation.
All three tools are **unverified live** under #97; there is no product deployment
or real Google mutation claim. Candidate totals: **181 / 143 active tools**,
**145 mapped native leaves**, **156 requiring method/scope/resource audit**.
The existing closed state of #96 is not completion evidence and is untouched.

## Selected-presentation read candidate (19 October 2026)

Three fixed-schema reads reuse native `slides info`, `slides read-slide` and
`slides locate`: `slides_info`, `slides_read_slide` and `slides_locate`. Each
uses one granted `presentation_id`; slide and page IDs remain local
selectors and never create additional resource grants. `slides_read_slide`
issues one native `Presentations.Get`; `slides_locate` uses the same one-get
model with UTF-16-aware literal matching locally. Optional CLI variants are
limited to `detail`, page, case, all/occurrence and fail-empty. There is no
regex, local-file access, downloads, unrelated external fetch or arbitrary request.

`slides_info` actually calls Drive Files.Get first, then Slides
Presentations.Get. It rejects anything that is not a Google presentation and
admits only `drive.file`; presentations scopes do not authorize the Drive call.
The other two reads admit `presentations.readonly`, `presentations` or
`drive.file` for one exact GET `/v1/presentations/<selected-id>`. Method/path
guards pin valid host/path pairs and deny cross-product requests. Official
Slides v1 and Drive v3 discovery checked 19 October 2026 supports these method
scopes.

Signed native fixtures use actual SDK types and guarded Slides plus Drive
fakes. Coverage includes split-run matching for `p:0` and `shape:one`, detail
output, default/all/occurrence/page/case locate behavior and permission,
tenant and invalid-selector denials before Google. D1/Clerk cases enforce the
selected encrypted `drive.file` connection, resource/tool isolation and foreign
connection denials. All three tools remain **unverified live** under #97; #96
remains pre-closed and untouched. No deployment, consent, credentials, Google
grants, billing configuration or ordinary CLI behavior changed.

Candidate totals: **178 tools / 140 active**, **142 mapped native leaves**,
**159 leaves requiring method/scope/resource audit**.

## Docs structure inventory candidate (19 October 2026)

Six fixed-schema reads reuse native `docs structure`, `headings list`,
`paragraphs list`, `tables list`, `images list` and `named-range list`:
`docs_structure`, `docs_headings_list`, `docs_paragraphs_list`,
`docs_tables_list`, `docs_images_list` and `docs_named_range_list`.
Only the selected `document_id` is exposed. Native default document views
are preserved; optional tab/all-tabs selectors and full CLI-option parity are
not claimed. Existing metadata and tab-list tool behavior is unchanged.

Each uses GET `/v1/documents/<selected-id>` with exact account/tool/file grants.
Official Docs v1 discovery checked 19 October 2026 admits
`documents.readonly`, `documents` or `drive.file`; broader Drive scopes are not
admitted. There are no content mutations, image downloads, external fetches or
automatic grants. Signed native fixtures exercise real structured outputs,
all three scopes and permission/tenant denials; D1/Clerk cases cover each tool.

Candidate totals: **175 tools / 137 active**, **139 mapped native leaves**,
**162 leaves requiring method/scope/resource audit**. All six tools remain
**unverified live** under #97. Missing-functionality scope #96 is not complete
despite its pre-existing closed state. No product deployment, consent,
credentials, Google grants, billing settings or ordinary CLI behavior changed.

## Selected-tab layout candidate (18 October 2026)

- `sheets_freeze` reuses native `sheets freeze` with an explicitly required sheet
  name and at least one integer `rows`/`cols` in 0–10,000. Zero unfreezes; omitted
  properties remain unchanged. No implicit first-tab fallback is exposed.
- `sheets_reorder_tab` reuses native `sheets reorder-tab`, with an explicit tab
  title/numeric ID and bounded destination index; native metadata preflight
  checks the destination against the current selected spreadsheet.

Both writes require exact enabled account/tool/file write authority and existing
native approval controls. Only `spreadsheets` or `drive.file` are admitted for
selected-spreadsheet metadata GET and POST `:batchUpdate`; readonly scopes fail
closed. Official Sheets v4 discovery checked 18 October 2026 confirms these
method scopes. Method/path guards deny POST metadata, GET batch, foreign files,
foreign hosts and other verbs. There is no deletion, duplication, arbitrary
request body or external/data-source operation.

Signed native stateful fixtures check actual persisted freeze properties, explicit
zero fields, preservation of omitted properties, and forward/backward tab order.
Missing tabs and invalid native indexes cannot mutate state. Both tools have
D1/Clerk file/tool isolation coverage. No production permission, credentials,
consent, billing setting or deployment changed; both tools remain **unverified
live** under #97. Candidate totals: **169 / 131 active tools**, **133 mapped
native leaves**, **168 requiring method/scope/resource audit**. #96/#97 stay open.

## Selected-spreadsheet inventory candidate (18 October 2026)

Five fixed-schema reads reuse native `sheets chart list`, `named-ranges list`,
`table list`, `banding list` and `conditional-format list`: `sheets_chart_list`,
`sheets_named_ranges_list`, `sheets_table_list`, `sheets_banding_list` and
`sheets_conditional_format_list`. Each accepts only `spreadsheet_id`, requires
an exact enabled Sheets tool/file grant and issues GET
`/v4/spreadsheets/<selected-id>` with the native fixed metadata field mask.
No arbitrary field masks, sheet/range selectors or mutation options are exposed.

Official Sheets v4 discovery checked 18 October 2026 admits
`spreadsheets.readonly`, `spreadsheets` or `drive.file`; broader Drive scopes are
not admitted. Signed native fixtures check actual field masks, returned metadata,
all three scope alternatives and grant/tenant denials. Each tool has a D1/Clerk
selected-file isolation test. All five remain **unverified live** under #97.
Candidate totals: **167 tools / 129 active**, **131 mapped native leaves** and
**170 leaves requiring method/scope/resource audit**. No deployment, consent,
credentials, Google grants, billing configuration or ordinary CLI changes.
#96/#97 remain open.

## Selected-file comments and form-watch read candidate (18 October 2026)

| Tool | Native command / Google method | Boundary | Live status |
| --- | --- | --- | --- |
| `drive_comments_list` | `drive comments list`; GET `/drive/v3/files/<id>/comments` | Exact selected-file/tool read grant; `drive.file` only; max 1–100, manual page cursor and optional quoted content; no all-pages/since or writes | Unverified live |
| `drive_comments_get` | `drive comments get`; GET `/drive/v3/files/<id>/comments/<commentId>` | Exact selected-file/tool read grant and canonical comment ID; `drive.file` only | Unverified live |
| `forms_watch_list` | `forms watch list`; GET `/v1/forms/<id>/watches` | Exact selected-form/tool read grant; `drive.file`, `forms.body.readonly`, `forms.body` or `forms.responses.readonly`; no watch creation, renewal, deletion or Pub/Sub target configuration | Unverified live |

Official Drive v3 and Forms v1 discovery documents checked 18 October 2026
confirm those method scopes; broader Drive scopes are not admitted. The candidate
has **162 total / 124 active tools**, mapping **126 native leaves**; **175 leaves
require method/scope/resource audit**. Catalogue/fixture coverage is not live
Google acceptance (#97). No deployment, consent, credentials, Google grants,
billing configuration or ordinary CLI behavior changed. #96/#97 remain open.

## Selected-document inventory candidate (18 October 2026)

- `docs_info` reuses native `docs info`: one GET `/v1/documents/<id>` with fixed
  `documentId,title,revisionId` fields.
- `docs_list_tabs` reuses native `docs list-tabs` (also `docs tabs list`): one GET
  of the selected document with `includeTabsContent=true`; output is tab IDs,
  titles, indexes and hierarchy, not a new arbitrary API proxy.

Both reads require an exact enabled Docs tool grant plus the enabled selected
file grant. Official Docs v1 discovery checked 18 October 2026 admits
`documents.readonly`, `documents` or `drive.file`; broader Drive scopes are not
admitted. Signed transport fixtures and D1 MCP permission tests cover selected
files, disabled/foreign resources, foreign tenants and scope/tool denials.
The candidate has **159 total / 121 active tools**, mapping **123 native leaves**;
**178 leaves require method/scope/resource audit**. Both tools are **unverified
live** under #97. No deployment, consent, credentials, grants or ordinary CLI
behavior changed. #96/#97 remain open.

## GA4 account inventory candidate (18 October 2026)

Two bounded native reads bring the candidate to **157 total tools / 119 active
priority tools**, mapping **120 native leaves**; **181 leaves still require
method/scope/resource audit**. This is fixed-schema coverage, not all CLI options.

- `analytics_accounts`: GET `/v1beta/accountSummaries` under an exact account-level
  tool grant. Returned accounts/properties do not acquire resource authority.
- `analytics_properties_list`: GET `/v1beta/properties` with exactly one fixed
  `parent:accounts/<id>` filter and an enabled selected-account resource grant.
  Arbitrary/ancestor/global filters, foreign accounts and request overrides fail
  closed; native page tokens retain the selected-account boundary.

The official Analytics Admin v1beta discovery document checked 18 October 2026
admits `analytics.readonly` or `analytics.edit` for both methods. Existing typed
handlers and signed transport are reused. No consent, deployed permissions,
credentials or deployment changed. Both tools remain **unverified live** under
#97; the two historical GA4 representative proofs are unchanged. #96 remains open.

## Additional blank Workspace creation candidate (18 October 2026)

Three further fixed adapters bring this candidate to **155 total tools / 117
active-priority tools**. They reuse native creation handlers and require an
explicit account-level exact tool/write grant; the caller cannot supply an
existing file, parent, template, import, image or publishing option.

| Tool | Native handler / actual Google call | Admitted scope | Live state |
| --- | --- | --- | --- |
| `docs_create_blank` | `DocsCreateCmd`; POST Drive `/drive/v3/files`, fixed document MIME type and name | `drive.file` only; `documents` alone does not authorize this Drive method | Fixture-tested, unverified live |
| `slides_create_blank` | `SlidesCreateCmd`; POST Drive `/drive/v3/files`, fixed presentation MIME type and name | `drive.file` only; `presentations` alone does not authorize this Drive method | Fixture-tested, unverified live |
| `forms_create_blank` | `FormsCreateCmd`; POST Forms `/v1/forms`, `info.title` only | `forms.body` or `drive.file` | Fixture-tested, unverified live |

Titles are bounded literal strings with Unicode control characters rejected.
Signed typed-HTTP fixtures check exact bodies, methods, endpoints, credentials,
scope alternatives and permission/input/tenant denials. D1-backed MCP fixtures
prove each creation leaves resource grants unchanged, rejects immediate reads
of returned IDs, and refuses foreign connections before dispatch. There is no
automatic grant, description update, publication action or ordinary CLI change.
No claim is made about a Google publication default beyond the actual API call.

The implementation matrix now maps **117 tools to 118 native leaves**, with
**183 leaves requiring actual method/scope/resource audit**; known exclusions,
product limitations and deferrals remain separate. All 117 candidate live rows
remain unverified; the two historical GA4 representative proofs are unchanged.
No Google consent, production permission, credentials, infrastructure or
deployment was changed for this candidate. #96/#97 remain open.

## Next bounded parity and evidence candidate (18 October 2026)

This candidate adds six tools beyond merged PR #100: **152 total tools across
23 services, 114 in the active 11-service priority set**. New tools are disabled
until their exact account/tool grants are saved. No Google consent, production
configuration, credentials or grants were changed, and no deployment occurred.

| Tool | Native command and actual Google methods | Authority / boundary | Live status |
| --- | --- | --- | --- |
| `drive_rename` | `drive rename`; selected-file PATCH | `drive.file`, explicit account/tool/file write permission | Fixture-tested, unverified live |
| `drive_copy` | `drive copy`; selected-source GET then POST `/copy` | `drive.file`; no destination parameter, returned file remains ungranted | Fixture-tested, unverified live |
| `drive_trash` | `drive delete` without permanent flag; fresh selected-file GET then PATCH `trashed:true` | `drive.file`, file/tool/write/destructive grants; reject folders, shortcuts, missing kind, failed verification and already-trashed resources before mutation | Fixture-tested, unverified live |
| `appscript_deployments` | `appscript deployments`; GET project deployments | Account-level exact tool grant; `script.deployments.readonly` or `script.deployments`, no project asset grant implied | Fixture-tested, unverified live |
| `appscript_versions` | `appscript versions`; GET project versions | Account-level exact tool grant; `script.projects.readonly` or `script.projects` | Fixture-tested, unverified live |
| `sheets_create_blank` | `sheets create`; POST `/v4/spreadsheets` | Account-level exact write-tool grant; `spreadsheets` or `drive.file`; bounded literal title only; no parent/template/source/custom tabs or automatic file grant | Fixture-tested, unverified live |

The 18 October official Drive, Apps Script and Sheets discovery documents
confirm these method scopes. Existing typed clients/handlers and request guards
are reused. Ordinary CLI behavior remains unchanged. The signed trash preflight
reuses the typed Picker verifier, retains the original caller/credential context,
and cannot turn a folder selection into authority over its unselected children.

`cli-hosted-matrix.json` now maps 114 hosted tools to 115 native leaves (one
Search Console alias). Tasks' 11 and GTM's 34 native leaves are already bound;
do not duplicate them. Another 186 native leaves require per-method/scope/resource
audit before eligibility is asserted; 21 are excluded, 20 product-limited and five
Drive-label leaves deferred. A mapped leaf is a fixed-schema subset, not blanket
native option parity. See [matrix generation](cli-hosted-matrix.md).

`live-tool-matrix.json` has one row per active candidate tool. Every candidate
row remains unverified live. Exactly two GA4 representative read variants have
attributable allow/deny evidence on the older deployed Worker version; unknown
source SHA remains null. Historical proof is not evidence that this candidate
was deployed or that a whole tool/service/variant family was exhaustively tested.

The infrastructure free-tier envelope does not include paid chat inference;
see [the cost boundary](free-tier-envelope.md). #96/#97/#69 remain open for their
respective missing implementation, live proof and external launch requirements.

## Current scope and Sheets engineering checkpoint (18 October 2026)

The active priority is **11 services**: Drive, Docs, Slides, Tasks, Sheets,
Forms, Apps Script, GA4, Search Console, BigQuery and GTM. **Calendar and Drive
Labels are now deferred**, in addition to the ten previous service deferrals
and earlier v2 exclusions. Preserve historical evidence below; do not treat the
older 13-service checkpoint as the current release scope.

This candidate adds four fixed-schema selected-spreadsheet tools, taking the
catalogue from 142 to **146 tools across 23 services**, including **108 tools
across the active 11 services** and nine Sheets tools. New tools remain disabled
until their exact account/tool permission is saved. Each uses the existing
selected-file resource grant; deletion additionally requires destructive
permission. No consent scope or production permission was changed.

| New hosted tool | Existing native command | Google methods | Destructive | Live state |
| --- | --- | --- | --- | --- |
| `sheets_add_tab` | `sheets add-tab` | POST spreadsheet `:batchUpdate` | No | Implemented, fixture-tested; unverified live |
| `sheets_rename_tab` | `sheets rename-tab` | GET spreadsheet; POST `:batchUpdate` | No | Implemented, fixture-tested; unverified live |
| `sheets_delete_tab` | `sheets delete-tab` | GET spreadsheet; POST `:batchUpdate` | Yes | Implemented, fixture-tested; unverified live |
| `sheets_number_format_range` | `sheets number-format` | GET spreadsheet; POST `:batchUpdate` | No | Implemented, fixture-tested; unverified live |

All four accept only `spreadsheets` or `drive.file` write authority. Current
[official Sheets discovery](https://sheets.googleapis.com/$discovery/rest?version=v4)
was checked on 18 October: `spreadsheets.get` and `spreadsheets.batchUpdate`
both accept these scopes. Restricted all-Drive alternatives are not admitted.
There is no arbitrary batch/format JSON proxy or spreadsheet creation in this
batch. Other native Sheets operations remain implementation work under #96;
these four newly executable tools enter #97 as **unverified live**.

The signed typed-HTTP fixture exercises add, metadata-backed rename, number
format and destructive deletion while preserving the original tab. Scope,
grant, selector, method/path and destructive-permission denials are tested.
This is deterministic engineering proof, not real Google mutation/read-back.
Separate real read-only proof for two existing GA4 tools is recorded in
[#97](https://github.com/Rajeev-SG/gog-marketing/issues/97#issuecomment-6251933114),
without claiming blanket service or catalogue verification.

## Historical priority and real acceptance (13 October 2026)

The owner defers **Contacts, YouTube, Photos, Groups, Chat, AdSense,
Classroom, Sites, Meet and People** to later roadmap steps. These ten services
are not current release-completion gates. Existing local CLI capabilities are
unchanged. The earlier Photos Picker/Cloud Admin/Keep/Workspace Admin v2
deferrals and Gmail/Google Ads/Drive Activity exclusions also remain unchanged.

The priority set is **13 services**. The historical 23-service engineering
matrix below is not the current release scope and must not override this update.

| Priority service | Real hosted evidence | Remaining boundary |
| --- | --- | --- |
| Calendar | Explicit selected holiday-calendar event listing succeeded | Event writes and other command families remain untested live |
| Drive | Real Picker selection and selected-file metadata reads; foreign-file denials | Wider eligible commands remain untested |
| Drive Labels | Missing consent repaired; live call reaches execution but fails | Workspace/customer prerequisite diagnosis must be confirmed live; not working acceptance |
| Docs | Task-created selected Doc append and independent exact-text read-back | Broader command parity and replacement coverage remain open |
| Slides | Task-created selected presentation listing and notes update; independent browser reload confirmed persisted notes | Broader command parity remains open |
| Tasks | Recorded real list/create/get/update/read-back/delete acceptance | Not full native command parity |
| Sheets | Recorded real write/read/clear/final-empty and resource/account denials | Broader command parity remains open |
| Forms | Task-created selected unpublished Form description update and independent read-back | Other mutations and response families remain untested live |
| Apps Script | Task-created blank project's metadata and source-content reads succeeded | Project creation through hosted MCP untested; function execution stays excluded |
| GA4 | Recorded real selected-property metadata read and mixed enabled/disabled-property batch | Broader Admin and write coverage remain untested live |
| Search Console | Real site list and selected owned-domain site get succeeded after scope consent repair | Query, inspection and sitemap mutation coverage remain open |
| BigQuery | Real customer-project dataset listing succeeded | Table/row coverage remains open; no SQL or billable query was attempted |
| GTM | Real selected-container metadata read succeeded | Live mutation/publishing acceptance remains open; existing containers were not changed |

The three new Docs/Slides/Forms files were created solely for acceptance and
selected through the real Google Picker. They started disabled. After testing,
all temporary file/tool grants (including destructive notes permission) were
restored disabled; the three files were moved to the owner's bin. The blank
Apps Script acceptance project was not executed. Foreign IDs were rejected by
real Docs/Slides/Forms MCP calls with `policy_denied / grant_denied`.

The restored shared stylesheet reintroduced a Google Picker collision: its
legacy `.picker` grid rule matched Google's `.picker-dialog`, collapsing the
real dialog to 266px. The candidate namespaces only the legacy local picker
selectors/template and regenerates the shared Worker CSS. Applying that exact
selector correction in the live browser restored a 1054px dialog and allowed
all three real file selections. Source checks passed; deployment verification
is still required before calling the deployed regression fixed.

The candidate also replaces a recognized Drive Labels valid-customer error
with a fixed `workspace_required` diagnostic, retaining generic errors for
other provider failures and never forwarding provider messages, credentials
or account data. Signed native-runner and Worker transport regressions cover
this boundary. This is not evidence of successful Drive Labels execution.

The OAuth app remains External/Testing. Positive second-account/non-owner
execution, full eligible native parity, remaining live writes, truthful legal
materials and applicable Google verification are still open. Representative
successes and scope deferrals do **not** establish public-beta readiness.

## Current milestone (12 October 2026)

This is an **engineering checkpoint, not public-beta readiness or live acceptance**.
The hosted-only consent policy contains 23 v1 service identifiers and an
exact OAuth scope allowlist. Local CLI scopes and capabilities are unchanged.
Photos Picker, Cloud Admin, Keep and Workspace Admin are deferred to v2 by the
owner on 12 October 2026 and are excluded from hosted v1 consent. Existing local
CLI functionality is unchanged. Drive Picker is separate and remains necessary
for authorizing pre-existing user files under `drive.file`.

Google production Data Access classifications were **console-verified and saved**
on 12 October 2026: 22 non-sensitive, 49 sensitive and zero restricted scopes,
including identity scopes. Brand/sensitive verification is **not approved**; the
existing app remains External/Testing with the two owner test accounts. Nothing here
proves the 23 services executable or working through hosted MCP.

The Worker now consumes a generated catalogue of one hundred and thirty-nine native Go
schemas: GA4 property/report reads and data-stream/key-event CRUD, custom-dimension/custom-metric management and GA4 Ads-link CRUD; selected-file Drive get, Docs
get/write, Sheets read/update, Slides slide listing/speaker-note replacement;
Calendar event listing/CRUD; eleven Tasks command leaves; all 34 existing native GTM account/container/workspace/tag/trigger/variable/version commands; eight Search Console tools; six Contacts tools; Drive label-schema list/get; Workspace Groups listing/member reads; Meet space create/get/update/end; AdSense account list/get; selected-file Sites metadata/URL reads; five selected Forms tools; Apps Script project get/content/create; partial Chat space/send tools; Classroom course CRUD; YouTube playlist read/create/delete; People self-profile read; app-created Photos list/get/search; and six BigQuery project-granted dataset/table metadata and bounded row reads. All invoke existing typed native handlers. MCP discovery and execution
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
binding and server-side typed Drive verification. Complete 23-service command
coverage and live verification remain required. Workspace bindings cannot access
an arbitrary file: an enabled verified selected-file grant must exist. Real
Picker use remains untested; the restricted key/project number are now configured.

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
| `people` | `profile` | No existing service write scope requested | **untested** | Native people_me uses account/tool grants, profile aliases and exact self-profile GET admission; unavailable People API fails without local credential fallback. Fixture reads/failure handling pass; arbitrary profiles/directory/relations/raw adapters and live acceptance pending | [Google](https://developers.google.com/people/v1/how-tos/authorizing) |
| `sheets` | `spreadsheets.readonly drive.file` | `spreadsheets` | **untested** | `sheets_read_range` and `sheets_update_range` guarded bindings implemented; Picker configuration/live acceptance and remaining command coverage pending | [Google](https://developers.google.com/workspace/sheets/api/scopes) |
| `forms` | `forms.body.readonly forms.responses.readonly drive.file` | `forms.body` | **untested** | Native selected-form get/update/publish and response list/get use verified file/tool grants; unpublish requires destructive permission. Picker MIME mapping and fixture persistence/denials pass. Creation/questions/watches and real acceptance pending | [Google](https://developers.google.com/workspace/forms/api/guides/apps-script-setup) |
| `sites` | `drive.file` | No existing service write scope requested | **untested** | Native sites_get/sites_url now use enabled Picker-selected file grants, drive.file and exact GET admission; fixture resource/scope denials pass. Global Drive-backed inventory/search excluded; multi-file URL authorization and live Picker/Sites acceptance pending | [Google](https://developers.google.com/workspace/drive/api/guides/api-specific-auth) |
| `meet` | `meetings.space.readonly` | `meetings.space.created meetings.space.settings` | **untested** | Native space create/get/update/end use account/tool grants, canonical space paths, method scopes and destructive end permission; fixture config persistence/scope denials pass. History/participants, account prerequisites and live acceptance pending | [Google](https://developers.google.com/workspace/meet/api/guides/authenticate-authorize) |
| `appscript` | `script.projects.readonly script.deployments.readonly script.processes` | `script.projects script.deployments` | **untested** | Native project get/content/create use account/tool grants, method scopes and exact project admission; fixture reads/create/denials pass. Function execution stays excluded pending target-scope audit; deployments/versions/bound-file/pull adapters and live acceptance pending | [Google](https://developers.google.com/apps-script/api/how-tos/execute) |
| `analytics` | `analytics.readonly` | `analytics.edit` | **untested** | Native property/report reads and data-stream/key-event CRUD, custom-dimension/custom-metric management and GA4 Ads-link CRUD routed with selected-property grants and method scopes; fixture persistence/scope/destructive/file-reference denials pass. Native custom-dimension/custom-metric list/get/create/update/archive and GA4 Google Ads-link CRUD are now included under analytics scopes, with destructive archive/delete grants. Bounded account-summary and selected-account property inventory are fixture-tested; live acceptance pending | [Google](https://developers.google.com/analytics/devguides/config/admin/v1/rest) |
| `searchconsole` | `webmasters.readonly` | `webmasters` | **untested** | Eight native site/query/sitemap/inspection tools routed with URL-prefix/domain site grants and method scopes; fixture submission/read-back/deletion and foreign URL/encoding denials pass. Cross-site sitemap authorization, richer query options and live acceptance pending | [Google](https://developers.google.com/webmaster-tools/v1/how-tos/authorizing) |
| `tagmanager` | `tagmanager.readonly` | `tagmanager.edit.containers tagmanager.edit.containerversions tagmanager.publish tagmanager.delete.containers` | **untested** | `tagmanager_versions_publish` uses the existing native handler, canonical enabled-container grants, publish scope and destructive permission; compiler failures are errors. Fixture execution/denial tests pass; all 34 existing native commands now have fixed bindings, exact selected account/container admission and method scopes; native fixture operations and denials pass; live GTM workflow acceptance pending | [Google](https://developers.google.com/tag-platform/tag-manager/api/v2/authorization) |
| `bigquery` | `bigquery` (combined scope; fixed GET-only runtime reads) | `bigquery` | **untested** | Six native dataset/table metadata/schema/row reads require an enabled customer project and exact GET endpoint admission; platform project and SQL jobs excluded. Dry-run/SQL execution and billing/destructive safeguards remain pending | [Google](https://docs.cloud.google.com/bigquery/docs/authentication) |
| `adsense` | `adsense.readonly` | No existing service write scope requested | **untested** | Native account list/get use account/tool grants, adsense.readonly and exact GET/canonical-account admission; fixture reads/scope/path denials pass. Remaining native AdSense resources/reports, publisher prerequisites and live acceptance pending | [Google](https://developers.google.com/adsense/management/reference/rest/v2/accounts/list) |
| `cloudadmin` | Not offered in v1 | Not offered in v1 | **v2 deferred** | Owner-deferred on 12 October 2026; excluded from hosted v1 consent and completion gates; local CLI preserved | [Google](https://docs.cloud.google.com/docs/authentication) |
| `groups` | `cloud-identity.groups.readonly` | No existing service write scope requested | **untested** | Both native groups_list/groups_members use account/tool grants, exact read-only scope, bound email lookup and validated membership paths; fixture reads/denials pass. Workspace role/live acceptance pending; no native writes claimed | [Google](https://developers.google.com/identity/protocols/oauth2/scopes) |
| `keep` | Not offered in v1 | Not offered in v1 | **v2 deferred** | Owner-deferred on 12 October 2026; excluded from hosted v1 consent and completion gates; local CLI preserved | [Google](https://developers.google.com/workspace/keep/api/reference/rest) |
| `admin` | Not offered in v1 | Not offered in v1 | **v2 deferred** | Owner-deferred on 12 October 2026; excluded from hosted v1 consent and completion gates; local CLI preserved | [Google](https://developers.google.com/workspace/admin/directory/v1/guides/authorizing) |
| `youtube` | `youtube.readonly` | `youtube` | **untested** | Native playlist list/create/delete use account/tool grants, mine selector, youtube write scope and destructive deletion permission; fixture persistence/read-scope denials pass. Remaining video/comment/channel/subscription/item features and live acceptance pending | [Google](https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps) |
| `photos` | `photoslibrary.readonly.appcreateddata` | No existing service write scope requested | **untested** | Native app-created list/get/search use exact library endpoints and approved scope/account/tool grants; fixture reads/legacy-scope/path denials pass. General-library browsing/writes excluded; download/Picker adapters and real app-created/account acceptance pending | [Google](https://developers.google.com/photos/overview/authorization) |
| `photospicker` | Not offered in v1 | Not offered in v1 | **v2 deferred** | Owner-deferred on 12 October 2026; excluded from hosted v1 consent and completion gates; local CLI preserved | [Google](https://developers.google.com/photos/picker/guides/get-started-picker) |

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
The production Console classification check now has direct saved evidence:
`/Users/rajeev/output/gog-marketing-parity/oauth-classification-20261012.png`.
The app remains External/Testing; free brand/sensitive verification and approved
legal/homepage configuration are still launch gates. Classification is not approval. Exact per-tool method/role
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

**Provisioned 12 October 2026 / not verified live:** website/API-restricted public
`GOG_GOOGLE_PICKER_API_KEY`, `GOG_GOOGLE_PICKER_APP_ID` (Google project number from
the same project as the central web OAuth client), Picker/Drive API enablement,
production origin and a real account's Google Picker consent. One key was created
in the existing project, restricted to Picker API and the existing HTTPS Worker
origin. Picker/API Keys APIs were enabled; Drive API was already enabled. Both
Picker bindings are stored as Worker secrets (the browser-visible key is not an
OAuth secret). No subscriptions, billing changes or paid assessments occurred. Missing prerequisites
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

People self-profile checked 12 October 2026 in [official People v1 discovery](https://people.googleapis.com/$discovery/rest?version=v1): people.get supports userinfo.profile. Existing approved basic profile scope aliases are accepted; no extra sensitive or restricted profile/directory scope is introduced. Real API enablement/account behavior remains unverified.

Photos library method scopes checked 12 October 2026 in [mediaItems.list](https://developers.google.com/photos/library/reference/rest/v1/mediaItems/list), [get](https://developers.google.com/photos/library/reference/rest/v1/mediaItems/get) and [search](https://developers.google.com/photos/library/reference/rest/v1/mediaItems/search): photoslibrary.readonly.appcreateddata is supported. Withdrawn general-library browsing is not introduced. Real app-created media/Google execution remain unverified.

BigQuery method scopes checked 12 October 2026 in the current [official BigQuery v2 discovery](https://bigquery.googleapis.com/$discovery/rest?version=v2): datasets.list/get, tables.list/get and tabledata.list list `bigquery`, not `bigquery.readonly`. Hosted consent uses `bigquery` with exact GET-only runtime admission; local CLI scope selection is unchanged. Fixture execution is not real Google acceptance.

GA4 customDimensions/customMetrics and googleAdsLinks method scopes checked
12 October 2026 in the official Analytics Admin v1beta discovery: reads allow
analytics.readonly or analytics.edit; mutations/archive require analytics.edit.
These are Analytics endpoints, not Google Ads API/adwords. Fixture CRUD/archive
persistence and scope/property/destructive/payload denials are not live evidence.

Review repairs: native GA4 Ads-link get now follows provider pagination; hosted
MCP schemas advertise a tenant-filtered connectionId enum, required for ambiguous
account-level tools. Account selectors are removed before native dispatch; foreign
accounts remain denied. Fixture page-two retrieval and schema-supported two-account
routing are regression evidence, not real Codex/Google acceptance.

Hosted BigQuery project discovery now uses the current typed BigQuery projects.list
API and paginates before selection. It excludes the platform-owned project,
deduplicates provider IDs and creates disabled project grants. No Cloud Platform
scopes, Cloud Admin operations, dataset creation or SQL jobs are introduced.
Existing local explicit-project discovery is unchanged. A paginated native HTTP
fixture verifies project grants/ownership; actual IAM/project availability remains
subject to real account acceptance.
