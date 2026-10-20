import { PRODUCT_CSS } from "./product-css.generated.js";

function escape(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
export function safeWorkspaceReturn(value: string | undefined): string {
  if (!value || !value.startsWith("/") || value.startsWith("//")) return "/";
  try {
    const url = new URL(value, "https://workspace.invalid");
    if (url.origin !== "https://workspace.invalid" || url.hash) return "/";
    if (url.pathname === "/" && !url.search) return "/";
    if (url.pathname !== "/chat") return "/";
    const keys = [...url.searchParams.keys()];
    if (!keys.length) return "/chat";
    const thread = url.searchParams.get("thread") ?? "";
    return keys.length === 1 && keys[0] === "thread" && /^[A-Za-z0-9_-]{1,128}$/.test(thread)
      ? "/chat?thread=" + encodeURIComponent(thread)
      : "/";
  } catch {
    return "/";
  }
}
export function accessDocument(title: string, content: string, scripts = ""): string {
  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${escape(title)} — gog-marketing</title><style>${PRODUCT_CSS}</style>${scripts}</head><body class="access-page"><a class="skip-link" href="#main-content">Skip to content</a><header class="access-nav"><a class="access-brand" href="/" aria-label="gog-marketing home"><span aria-hidden="true" class="brand-mark">g</span>gog-marketing</a><nav aria-label="Public navigation"><a href="/about">About &amp; privacy status</a><a class="button secondary-action" href="/sign-in">Sign in</a></nav></header>${content}<footer class="access-footer"><span>Private beta · Access stays under your control.</span><a href="/about">About &amp; privacy status</a></footer></body></html>`;
}
export function renderLanding(): string {
  return accessDocument(
    "A workspace for your next campaign",
    `<main id="main-content" tabindex="-1" class="access-main"><section class="landing-hero"><div><p class="eyebrow">Your marketing workspace, connected</p><h1>Less moving data.<br>More moving work forward.</h1><p class="access-lead">Bring Google Docs, Sheets, Slides and Drive into a governed AI workflow. Turn a brief into a first draft, make sense of campaign results, and prepare the next client handoff.</p><div class="access-actions"><a class="button primary" href="/sign-in">Sign in to your workspace <span aria-hidden="true">→</span></a><a href="#how-access-works">See how access works</a></div><p class="access-caption">Invitation-only beta. No Google account is connected and nothing is changed just by signing in.</p></div><aside class="workflow-preview" aria-labelledby="workflow-heading"><p class="eyebrow">A clearer handoff</p><h2 id="workflow-heading">From scattered files<br>to a useful next step.</h2><ol><li><strong>Start with the brief</strong><span>Choose the documents and campaign files that matter.</span></li><li><strong>Work through the detail</strong><span>Use chat to draft, analyse or prepare a presentation.</span></li><li><strong>Review before it changes</strong><span>Google writes need your explicit approval.</span></li></ol><p class="faint">Example workflow · availability follows your saved permissions.</p></aside></section><section id="how-access-works" class="access-explainer" aria-labelledby="access-heading"><div><p class="eyebrow">No blanket access</p><h2 id="access-heading">You choose the account,<br>the data, and the tools.</h2></div><ol class="access-steps"><li><strong>1. Connect deliberately</strong><p>Choose a Google account and only the services you want to use.</p></li><li><strong>2. Select the right data</strong><p>Pick files and resources. Google consent alone does not enable assets or AI tools.</p></li><li><strong>3. Stay in the review loop</strong><p>Read access and changes are separate. Destructive actions need a further opt-in.</p></li></ol></section><section class="access-invite"><div><h2>Already invited?</h2><p>Your existing sign-in takes you to your own workspace. Looking for access? Ask your workspace administrator for an invitation.</p></div><a class="button secondary-action" href="/request-access">About beta access</a></section></main>`,
  );
}
export function renderRequestAccess(): string {
  return accessDocument(
    "Private beta access",
    `<main id="main-content" tabindex="-1" class="access-main access-state"><div><p class="eyebrow">Invitation-only beta</p><h1>A useful workspace starts<br>with the right access.</h1><p class="access-lead">Ask your workspace administrator to invite the account you use for work. An invitation and your saved permissions determine which private features are available.</p><p class="access-notice" role="note">No request has been submitted. This page does not send email or create an account.</p><div class="access-actions"><a class="button primary" href="/sign-in">I already have an invitation</a><a href="/">Back to the product</a></div></div></main>`,
  );
}
export type AccessState = "not-invited" | "expired" | "unavailable";
export function renderAccessState(kind: AccessState, returnTo = "/chat"): string {
  const path = returnTo === "/sign-in" ? "/sign-in" : safeWorkspaceReturn(returnTo);
  const copy =
    kind === "not-invited"
      ? {
          eyebrow: "Private beta",
          title: "You’re signed in.<br>This workspace isn’t available yet.",
          body: "This account is not currently invited to chat, or its workspace access has been paused. Your sign-in does not grant access to another account’s conversations or Google data.",
          primary: "About beta access",
          href: "/request-access",
          secondary: "Use another account",
          other: "/sign-in?switch=1&returnTo=" + encodeURIComponent(path),
        }
      : kind === "expired"
        ? {
            eyebrow: "Session ended",
            title: "Sign in again to<br>pick up where you left off.",
            body: "Your session has expired or was signed out. Existing conversations and your tab-local draft are kept; access must be verified again before continuing.",
            primary: "Sign in again",
            href: "/sign-in?expired=1&returnTo=" + encodeURIComponent(path),
            secondary: "Back to the product",
            other: "/",
          }
        : {
            eyebrow: "Temporarily unavailable",
            title: "We couldn’t verify<br>your workspace right now.",
            body: "The sign-in or workspace service could not be reached. No new access was granted and your existing selections were not changed. Please retry in a moment.",
            primary: "Try again",
            href: path,
            secondary: "Back to the product",
            other: "/",
          };
  return accessDocument(
    copy.eyebrow,
    `<main id="main-content" tabindex="-1" class="access-main access-state"><div><p class="eyebrow">${copy.eyebrow}</p><h1>${copy.title}</h1><p class="access-lead">${copy.body}</p><div class="access-actions"><a class="button primary" href="${escape(copy.href)}">${copy.primary}</a><a href="${escape(copy.other)}">${copy.secondary}</a></div></div><aside class="access-state-note"><strong>Your data stays scoped.</strong><p>Signing in, changing accounts or retrying a page never enables Google assets, tools or writes.</p></aside></main>`,
  );
}
