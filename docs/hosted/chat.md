# Integrated owner chat

Chat is part of gog-marketing at `/chat`, using the same Clerk sign-in as
Google account settings. This milestone is an owner-only chatbot, **not a
Google-connected agent**. The interface says “Google tools are not connected
yet.” Existing Google connections and permissions remain managed in the
original product interface.

## Implementation and boundaries

The imported UI/runtime comes from `Rajeev-SG/gog-marketing-chat` PR #3,
commit `3825044ce66dd201eacc527b61af18243e5cae22`, including its UX fixes.
React + assistant-ui use Cloudflare's native `useAgentChat`/`AIChatAgent`;
AI SDK invokes OpenRouter server-side. No parallel Google client, chat store,
agent framework, or model-provider browser credential is added.

The existing `gog-marketing` Worker serves the product, `/chat`, the chat
asset bundle, and protected agent history/WebSocket routes on the same origin.
A new SQLite-backed `Chat` Durable Object owns the fresh conversation.
The existing D1 database remains authoritative for Clerk-to-tenant identity
and Google connections. Chat transcripts are not duplicated into D1.

`CHAT_OWNER_USER_ID` must name the verified Clerk owner in the deployed Clerk
instance. Missing configuration fails closed. Other users, anonymous requests,
foreign conversations, inactive tenants, expired/revoked sessions and untrusted
origins cannot access the conversation or start inference. Clerk authentication
and live session status checks happen server-side. No browser-supplied identity
can select another conversation. Drafts are tab-local and account-scoped.

The model defaults to `openai/gpt-4.1-mini`. Limits remain 10 model starts per
minute, 100 per UTC day, 100 KB conversation, 8,000 characters per user message,
2,048 output tokens, and a 90-second response timeout. Limits are enforced in
the integrated conversation store, not shared with the legacy standalone app.
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
than claiming the server session was revoked.

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
`CHAT_OWNER_USER_ID`/`OPENROUTER_MODEL`, preserving existing Worker variables,
and deploying. Never copy `.dev.vars`, `.owner-access.txt`, credentials or
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

## Next milestone — Google tools

Connect chat to gog-marketing's existing authenticated hosted MCP surface.
Use the signed-in user's current service/tool/resource permissions, not a
separate Google login or a generic CLI/HTTP proxy. Prove a useful read workflow
and an explicitly approved write followed by read-back. Disabled access must
remain denied. Recheck the imported MCP dependency advisory before enabling
any MCP/OAuth client functionality.

The broader agent epic remains future work: self-service provider keys,
billing, memory, task management and execution receipts do not block this
owner-chat integration. No full CLI command parity is required for this step.
