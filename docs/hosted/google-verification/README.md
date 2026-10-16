# Google verification preparation — #94 / #79 / #69

Checked **16 October 2026**, using the existing `gog-marketing-prod` project and
Google Auth Platform console. This packet is preparation, **not Google approval**
and not a statement that public beta is ready.

## Live console audit

- Audience: **External / Testing**, two existing test users, no public users.
- Branding app name: `gog-marketing`. Existing support/developer contact is the
  owner's configured address; no new identity or legal entity has been invented.
- Application homepage, privacy-policy and terms URLs were **blank** at the audit.
  This implementation provides a public descriptive `/about` page; configure its
  URL only after its deployed anonymous200 check. Approved policy/terms pages
  remain an owner/legal gate, not links to these drafts.
- Authorized domain: `rajeev-sgill.workers.dev`. Search Console ownership proof
  still needs to be verified; merely appearing here is not proof of ownership.
- Verification centre says verification is not required **because the app is
  in Testing**. That is not production approval. Audience warns that Branding
  must be completed before publishing.
- Project checkup warns about the number of project owners/editors. Do not add a
  second administrator without the owner's choice; confirm whether the warning
  blocks submission or is a resilience recommendation.
- Initial Data Access inventory: 22 non-sensitive, 49 sensitive, **zero restricted**
  scopes. The submission set has been narrowed to the current 13-service consent
  surface plus canonical identity scopes: **33 scopes (6 non-sensitive, 27
  sensitive, zero restricted)**, saved and verified after reload. See
  `scope-packet.json`. The 38 deferred-service scopes were removed from the
  console submission list, not from stored grants or the preserved CLI catalogue.
- No publishing switch, verification submission, legal acceptance, client secret,
  redirect URI, connection credential, paid assessment or new infrastructure was
  changed by this preparation.

## Ready-to-use materials

1. `scope-packet.json`: exact scope-to-service mapping and observed console
   classifications. This follows current executable policy, not a guess from
   “read-only” labels. `drive.file` stays selected-file-only; Gmail, Ads,
   broad Drive/Drive Activity, Cloud Admin, Keep, Workspace Admin and Photos
   Picker are not included.
2. `privacy-draft.md`: factual data-flow copy for owner/legal review. It explicitly
   discloses OpenRouter processing, transcript retention and current deletion
   boundaries. It is **not published or represented as approved policy**.
3. `terms-draft.md`: private-preview terms draft, without invented jurisdiction,
   business identity, contractual promises or paid services.
4. `demo-script.md`: reviewer walkthrough and capture/redaction checklist.

The owner-only app, original conversation and standalone deployment remain
available. Existing Google connection credentials are not re-consented or
revoked as a shortcut for verification filming.

## Factual submission description

> gog-marketing lets users connect their Google accounts and select the services,
> exact tools and resources they want to use. Its settings interface and hosted
> MCP tools enforce those choices server-side. The private chat interface uses
> the same permissions. Read requests retrieve data for user-requested work;
> writes require a separate review of exact arguments before execution. Users
> can disconnect Google accounts. Google credentials never go to the language
> model. Google tool results sent to chat may be processed by OpenRouter and the
> selected model provider, which is disclosed to the user.

## What still needs genuine approval or external evidence

- Approve operator identity/contact, the privacy/terms drafts, retention and the
  stated AI-processor disclosure. Do not turn draft wording into a legal
  attestation on the owner's behalf.
- Choose/confirm the final public product origin and verify ownership in Search
  Console. A custom owned domain is an option, not a new purchase requirement.
  Complete a public descriptive homepage and approved HTTPS policy/terms pages;
  do not point Google at blank, private, placeholder or draft-only pages.
- Decide whether the 13-service release really needs every configured broad
  sensitive scope. In particular, review whether selected Docs/Sheets/Slides/
  Forms methods can use only `drive.file`; narrow *new* consent without rejecting
  already stored grants. Do not claim broad file scopes are the narrowest merely
  because server-side resource checks are strong.
- Supply the final unlisted demo-video URL. Capture actual Google consent and
  all scope families being submitted; existing session success is not footage
  of fresh consent. Do not revoke current owner credentials just to film it.
- Provide a reviewer-access path/test account that does not expose the owner's
  private data. Public/non-owner chat admission remains separate beta work;
  do not flip the owner gate globally to let a reviewer in.
- After those items: complete Branding, publish the intended audience when
  appropriate, submit sensitive-scope verification, answer Google's questions,
  and record the actual outcome. No paid CASA work is authorized. If Google
  classifies a requested scope as restricted, exclude it pending a separate
  decision rather than purchasing an assessment.

## Authoritative references checked

- [Sensitive-scope verification and exceptions](https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification)
- [Brand verification](https://developers.google.com/identity/protocols/oauth2/production-readiness/brand-verification)
- [Google API Services User Data Policy / Limited Use](https://developers.google.com/terms/api-services-user-data-policy)
- [OAuth production-readiness checklist](https://developers.google.com/identity/protocols/oauth2/production-readiness)
- [OpenRouter data handling](https://openrouter.ai/docs/guides/privacy/data-collection)

The backend requests `data_collection: deny` and `zdr: true` for chat inference;
if no eligible endpoint exists it fails rather than silently relaxing routing.
That request-level setting is not proof of every workspace/account-level logging
setting or a substitute for reviewing the provider's contracts and policy.
