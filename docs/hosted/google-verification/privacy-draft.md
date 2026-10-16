# Privacy information — DRAFT FOR OWNER REVIEW

Not published, not legal advice and not an approved privacy policy. Operator
identity, final URL/contact, retention and contractual obligations must be
confirmed before this is used in Google verification.

## Data and purposes

gog-marketing uses Clerk for product sign-in. Connecting Google is separate:
it verifies the Google account identity and asks only for the Google services
and read/write access selected in settings. The app stores account identity,
consent metadata, encrypted Google credentials, resource/tool grants and audit
metadata to provide the requested account operations and enforce permissions.

Google API requests use the selected account and the enabled tool/resource.
Google access/refresh credentials are not given to the model or stored in chat
messages. Server-side code may obtain short-lived Google access tokens solely
to invoke the existing private execution service.

When chat invokes a Google tool, the user-requested tool result and necessary
account/resource context can be included in messages sent to OpenRouter and the
configured model provider. Do not use chat with data you are not authorized to
share with those processors. Chat requests require provider data-collection
opt-out and zero-data-retention routing; service-provider policies and any
workspace-level optional logging must also be reviewed before public launch.
The application does not implement model training on Google data.

## Storage, deletion and controls

Cloudflare hosts the application, D1 account/permission metadata, and Durable
Object conversations. Google refresh credentials are encrypted at rest. Chat
messages, tool arguments/results and pending approvals are saved so users can
resume conversations. Drafts are stored in the current browser tab and scoped
to the account and conversation. Deleting a conversation cancels its pending
work and removes its server transcript/approvals; the current tab's associated
draft is cleared. A minimal deletion marker is retained to prevent resurrection
and support retrying interrupted cleanup. Do not promise instant deletion from
third-party backups or other browser tabs.

Disconnecting a Google account uses the application's revocation/deletion flow.
It does not automatically erase previously saved chat messages containing prior
Google results; delete those conversations separately. Whole-product/Clerk
account deletion, audit retention and third-party backup handling must be
finalized before public beta. No numerical retention period has been invented.

## Sharing and human access

Processing is limited to requested product features, necessary service providers,
security/legal obligations and the user's approved operations. Do not sell Google
data, use it for advertising, or permit unrelated/generalized model training.
Human access, if required for support/security/legal duties, must be limited and
disclosed under Google's applicable Limited Use requirements.

Before publication, the owner must approve a Google API Services User Data Policy
and Limited Use compliance statement and verify that operational/provider
settings actually support it. This draft is not that attestation.
