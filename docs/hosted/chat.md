# Integrated private chat

Chat is part of gog-marketing at `/chat`, using the same Clerk sign-in as
Google account settings. Chat remains owner-only unless the operator explicitly
configures the first verified cohort. It can now invoke a bounded starter set of
existing Google tools, only when the account, exact tool and
resource are enabled in the original Google settings. Nothing is enabled merely
by signing in. Every write requires a separate approval card.

## Implementation and boundaries

The imported UI/runtime comes from `Rajeev-SG/gog-marketing-chat` PR #3,
commit `3825044ce66dd201eacc527b61af18243e5cae22`, including its UX fixes.
React + assistant-ui use Cloudflare's native `useAgentChat`/`AIChatAgent`;
AI SDK invokes OpenRouter server-side. No parallel Google client, chat store,
agent framework, or model-provider browser credential is added.

The existing `gog-marketing` Worker serves the product, `/chat`, the chat
asset bundle, and protected agent history/WebSocket routes on the same origin.
The existing `Chat` Durable Object namespace now stores one independent native
conversation per thread. The original tenant-derived object and draft key remain
unchanged as “Original conversation”; no transcript is copied or migrated.
A tenant-scoped D1 index (`0007_chat_threads.sql`) stores only thread identity,
title and timestamps. Separate DOs isolate messages, streams and approvals.
The existing D1 database remains authoritative for Clerk-to-tenant identity
and Google connections. Chat transcripts are not duplicated into D1. Shared
model-start quotas are account-wide in D1; creating/deleting a thread cannot
multiply allowances, and existing legacy counters are carried forward.

`CHAT_OWNER_USER_ID` must name the verified Clerk owner in the deployed Clerk
instance. Missing owner configuration fails closed. For the first cohort only,
operators may set `CHAT_COHORT_USER_IDS` to a comma-separated list of exact
Clerk user ids; omit or leave it empty for owner-only chat. A non-empty list must
contain only valid, duplicate-free `user_*` Clerk ids. A malformed list is a hard
chat outage, never an implicit fallback. Allowlisting only admits the user: each
request still needs a server-derived active D1 tenant, a live Clerk session, and
a tenant-owned conversation before HTML, metadata, threads, history, sockets,
inference or tools. There is no open-signup inference. Other users, anonymous
requests, foreign conversations, inactive tenants, expired/revoked sessions and
untrusted origins cannot access the conversation or start inference. Clerk
authentication and live session status checks happen server-side. No
browser-supplied identity can select another conversation. Drafts are tab-local
and account-scoped.

The model defaults to `openai/gpt-4.1-mini`. Limits remain 10 model starts per
minute, 100 per UTC day, and 1,000 per UTC calendar month per tenant; creating
threads or changing sessions cannot multiply those allowances. Current legacy
daily usage is conservatively carried into the new monthly counter. Requests
denied by a later budget may still consume earlier budget reservations; denial
never authorizes inference. Other limits remain 100 KB conversation, 8,000 characters per user message,
2,048 output tokens, and a 90-second response timeout. Text limits are enforced per native conversation; model-start limits are enforced
account-wide in D1, separate from the legacy standalone app.
Provider failures are redacted. Native streaming, stop, reconnect and saved
messages are retained. Recipient sessions are rechecked before native outbound
batches, including idle sockets; expired/revoked recipients are closed before
new conversation data is delivered. Delivery is coalesced to at most one fresh
recipient check per second, respecting Clerk's development-instance limit of
[100 Backend API requests per 10 seconds](https://clerk.com/docs/guides/how-clerk-works/system-limits#backend-api-requests).
This is delivery batching, not a positive-authorization cache: each batch is
checked immediately before release. Temporary auth outages/rate limits close
sockets with retryable code 1013 instead of falsely claiming sign-out.
This bounded, in-memory authorization buffer
does not replace the SDK protocol or store a second transcript. Size limits use
UTF-8 bytes, and stopped-message annotations retain the full conversation snapshot.
A failed Clerk sign-out closes the local chat but shows an explicit retry rather
than claiming the server session was revoked. If the SDK stops reconnection after
a terminal close, a new successful owner-session check may restart its native
transport. This retry is throttled; a 401/403 instead unmounts chat, and no retry
uses a stale cached permission result. Clerk identity changes invalidate in-flight
metadata responses before rechecking, so a late response cannot revive the prior
account's chat or unsent draft.

## Build and operate

From `internal/hosted/app`:

```sh
pnpm install --frozen-lockfile
pnpm typecheck
pnpm build
pnpm test
pnpm dev
```

The build creates only public frontend files in ignored `dist/client`.
The Worker serves authenticated chat HTML; requests to APIs and agent routes
must never fall back to the frontend's HTML. Existing Google OAuth callback,
connection, grant and external `/mcp` routes retain their original behaviour.

See the operator runbook for provisioning `OPENROUTER_API_KEY`, setting
`CHAT_OWNER_USER_ID`/`OPENROUTER_MODEL`, optionally setting
`CHAT_COHORT_USER_IDS`, preserving existing Worker variables, and deploying.
Never copy `.dev.vars`, `.owner-access.txt`, credentials or
standalone conversation data into this repository or its public assets.

## Verification and work tracking

Integration and its acceptance evidence are tracked in
[issue #90](https://github.com/Rajeev-SG/gog-marketing/issues/90).
Implementation, deterministic test results and real deployed acceptance are
separate claims; a successful build alone does not establish a working login
or model response. Record real verification against the deployed commit before
closing the integration issue.

GitHub prevents transferring issues from the private source repository to this
public repository. The original discussions remain intact in the source repo;
no private issue discussion is published as a workaround without approval.
The source app and its original saved conversation remain privately available.
No Durable Object transfer or deletion is part of this integration.

## Google tools and approvals (#92)

`src/chat/tools.ts` selects a bounded starter surface (`CHAT_GOOGLE_TOOLS`), not
full CLI parity. The model first calls `google_access` to see enabled accounts,
exact tools and selected resource IDs. Unavailable services are not advertised.
The same `policyTools` and `handleToolCall` functions used by external `/mcp`
resolve current grants, enforce quotas, acquire ephemeral Google credentials,
audit and invoke the existing fixed private runner mappings. No parallel Google
client, generic HTTP/CLI proxy, new bearer token or duplicate permissions UI exists.
Every execution rechecks the live Clerk session, then the existing MCP path
rechecks account/tool/resource authority, including changes made after approval.

AI SDK 7's native `toolApproval` flow and assistant-ui tool cards provide inline
**Approve Google action / Don't run** controls, showing exact account/resource
arguments. A separate server-side Durable Object approval binds the generated
call ID, owner, tenant, tool and canonical arguments. It expires after 10 minutes;
authenticated approval is one-use and is consumed before dispatch. Denied,
expired, changed or replayed approvals cannot execute. Browser-invented approvals
or tool results cannot authorize Google actions. Approved calls may resume through
the native SDK after reload without losing the gate. If a write's outcome is
uncertain, it must not be automatically retried. After success the model uses an
available read tool to verify, otherwise it explicitly reports that verification
is unavailable. Stop cancels further dispatch, but cannot roll back a write that
has already reached Google.

The 10/minute, 100/UTC-day and 1,000/UTC-month model-start budgets, 100 KB conversation cap,
2,048-token per-step output limit and 90-second run timeout remain. A run is bounded
to six model/tool steps; each approval continuation is authenticated and budgeted.
Saved transcripts include tool arguments/results: use only accounts/data you are
comfortable passing to the configured OpenRouter model.

The installed production dependency audit was rechecked: the MCP client/SDK
OAuth credential-forwarding advisory remains in transitive dependencies. This
bridge does **not** instantiate an MCP/OAuth client or connect to a remote MCP
server; no OAuth client credentials are handed to an MCP-selected issuer. Do not
enable remote MCP/OAuth client functionality without resolving that advisory.
Other existing dependency advisories were not silently upgraded in this change.

Implementation/fixture proof and real Google acceptance are recorded separately
in [issue #92](https://github.com/Rajeev-SG/gog-marketing/issues/92). A passing fixture
is not a claim of a real approved write. Existing Google permissions are never
changed as a testing shortcut.

Self-service provider keys, billing, memory, broader task management and additional
command coverage remain later work.


## Workspace navigation and threads (#94)

Settings and Chat use the same canonical navigation destinations and generated
product-shell stylesheet. Desktop navigation includes Home, Google data, Chat,
Activity and Settings; mobile navigation retains those destinations and a
keyboard-accessible conversations drawer. Links back to settings use absolute
product paths, not chat-local hash fragments.

`GET/POST /api/chat/threads`, `PATCH/DELETE /api/chat/threads/:id` and
`GET /api/chat/session?thread=:id` derive ownership from the verified live Clerk
owner. Mutation requests require the trusted same-origin Origin and JSON content
type. Titles are plain text, 1–100 characters. Up to100 active threads are allowed.
New threads get server UUIDs and opaque server-derived conversation/draft keys;
clients cannot choose another owner's namespace. The original conversation is
registered lazily without changing its historical native object or draft key.

New chat, thread links/back-forward/reload, automatic first-message titles,
rename and confirmed deletion are supported. Drafts are tab-local and independent
per account/thread. Deletion tombstones the owner index before native destruction,
blocks subsequent history/upgrades/messages/model/tools immediately, cancels native
work and removes transcript/approval storage. A failed purge returns a retryable
error, not a false success. Pending cleanup stays discoverable in the owner sidebar
across rechecks/reloads; Finish deletion retries that same immutable ID. Deleted
bookmarks recover to surviving/new owner threads instead of trapping the workspace.
The pending list is served in batches of100; remaining items appear as cleanup
completes. Minimal opaque tombstones remain to prevent resurrection;
deleted titles are removed. Other threads and standalone storage are untouched.
An already-executed Google operation cannot be rolled back by deleting its chat.

Thread deletion is reached only through the authenticated product API. Native
budget/purge suffixes are internal namespace requests and are never routed from
public `/agents` URLs or exposed as browser-callable methods.

Google verification preparation is in [google-verification/README.md](google-verification/README.md).
It distinguishes checked console facts, prepared drafts and genuine owner/external
gates. Do not call Testing status or draft legal text Google production approval.
