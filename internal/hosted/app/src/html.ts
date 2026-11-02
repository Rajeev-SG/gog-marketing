import { FAVICON_LINK, accessDocument, safeWorkspaceReturn } from "./access-pages.js";
export { renderLanding, renderRequestAccess, renderAccessState } from "./access-pages.js";
import { PRODUCT_CSS } from "./product-css.generated.js";

/** HTML shell rendering for the canonical hosted customer UI. */
export interface HtmlConfig {
  publishableKey: string;
  authorizedParties: string[];
  clerkDomain: string;
  mcpOrigin: string;
}

/** Derive the Clerk Frontend API domain from a publishable key. */
export function deriveClerkDomain(publishableKey: string): string {
  try {
    const parts = publishableKey.split("_");
    if (parts.length < 3 || !parts[2]) return "";
    const domain = atob(parts[2]).slice(0, -1);
    return isValidClerkDomain(domain) ? domain : "";
  } catch {
    return "";
  }
}

function isValidClerkDomain(domain: string): boolean {
  return /^[a-z0-9][a-z0-9.-]*\.[a-z]{2,}$/.test(domain);
}

function escapeHtmlAttr(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** Baseline CSP follows Clerk's manual CSP guide and existing hosted policy. */
export function securityHeaders(clerkDomain = ""): Record<string, string> {
  const clerkSources = clerkDomain ? [`https://${clerkDomain}`, `wss://${clerkDomain}`] : [];
  const clerkScriptSources = clerkDomain ? [`https://${clerkDomain}`] : [];
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline' ${clerkScriptSources.join(" ")} https://challenges.cloudflare.com https://*.protect.clerk.com https://apis.google.com`,
    "style-src 'self' 'unsafe-inline'",
    `img-src 'self' data: ${clerkScriptSources.join(" ")} https://img.clerk.com`,
    `font-src 'self' ${clerkScriptSources.join(" ")}`,
    `connect-src 'self' ${clerkSources.join(" ")} https://*.protect.clerk.com:*`,
    `frame-src ${clerkScriptSources.join(" ")} https://challenges.cloudflare.com https://*.protect.clerk.com https://docs.google.com`,
    "worker-src 'self' blob:",
    `form-action 'self' ${clerkScriptSources.join(" ")}`,
    "object-src 'none'",
    "base-uri 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
  return { "Content-Security-Policy": csp, "X-Content-Type-Options": "nosniff" };
}

export function clerkScriptTags(htmlConfig: HtmlConfig): string {
  const pk = escapeHtmlAttr(htmlConfig.publishableKey);
  return `<script defer crossorigin="anonymous" type="text/javascript" src="https://${htmlConfig.clerkDomain}/npm/@clerk/ui@1/dist/ui.browser.js"></script>
  <script defer crossorigin="anonymous" type="text/javascript" data-clerk-publishable-key="${pk}" src="https://${htmlConfig.clerkDomain}/npm/@clerk/clerk-js@6/dist/clerk.browser.js"></script>`;
}

export function renderSignIn(
  htmlConfig: HtmlConfig,
  returnTo = "/",
  switchAccount = false,
): string {
  const target = safeWorkspaceReturn(returnTo);
  const mountOptions = JSON.stringify({
    fallbackRedirectUrl: target,
    appearance: {
      variables: {
        colorPrimary: "var(--action)",
        colorForeground: "var(--text)",
        colorBackground: "var(--surface-card)",
        fontFamily: "inherit",
        borderRadius: "6px",
      },
      elements: {
        rootBox: { width: "100%" },
        cardBox: { width: "100%", boxShadow: "none", border: 0, overflow: "visible" },
        // Padding keeps input/button borders inside the box (no clipped edges).
        card: { boxShadow: "none", border: 0, padding: "2px" },
        // The page already titles the panel; avoid "Sign in" / "Welcome back" duplicates.
        header: { display: "none" },
        footerAction: { display: "none" },
        formButtonPrimary: {
          backgroundColor: "var(--action)",
          backgroundImage: "none",
          boxShadow: "none",
          borderRadius: "6px",
          minHeight: "44px",
          fontWeight: 600,
        },
        socialButtonsBlockButton: { borderRadius: "6px", minHeight: "44px" },
        formFieldInput: { borderRadius: "6px", minHeight: "44px" },
        // Header is hidden; keep bounded typography in case Clerk shows it (#144).
        headerTitle: {
          fontSize: "1.25rem !important",
          lineHeight: "1.4 !important",
          letterSpacing: "normal !important",
          margin: "0 !important",
        },
        headerSubtitle: { fontSize: "0.875rem", lineHeight: "1.5" },
        // Keep the measured auth-strategy badge inside the provider card edge (#145).
        lastAuthenticationStrategyBadge: {
          right: "8px",
          top: "0",
          transform: "translateY(-50%)",
        },
      },
    },
  }).replaceAll("<", "\\u003c");
  const scripts = clerkScriptTags(htmlConfig);
  const content = `<main id="main-content" tabindex="-1" class="access-main auth-layout"><section class="auth-intro"><p class="eyebrow">gog-marketing workspace</p><h1>Sign in to your workspace</h1><p class="access-lead">Sign in to connect the Google accounts and files you choose, then work with AI under your saved permissions.</p><ul class="auth-promises"><li>Separate read and change permissions</li><li>Selected files, not blanket access</li><li>Explicit review before Google writes</li></ul><a href="/request-access">Need a beta invitation?</a></section><section class="auth-panel" aria-labelledby="signin-heading"><h2 id="signin-heading">Use your invited account</h2><p class="muted">Sign in with the work account your administrator invited.</p><div id="sign-in"><p role="status">Secure sign-in is loading…</p></div><div id="signin-error" hidden role="alert"><h3>Sign-in is temporarily unavailable</h3><p>We could not load the sign-in service. No access was changed. Check your connection, then retry.</p><button type="button" class="primary" id="signin-retry">Retry sign-in</button></div><div id="switch-account" hidden><p>You are already signed in. Sign out before using another account.</p><button type="button" class="primary" id="switch-signout">Sign out to switch account</button></div><p class="access-caption">Google sign-in stays with Google. Access tokens are stored encrypted; signing in alone does not enable assets or tools.</p></section></main><script>(function () { var target=document.getElementById('sign-in');var failure=document.getElementById('signin-error');function fail(){target.hidden=true;failure.hidden=false;}var timedOut=false;var deadline=setTimeout(function(){timedOut=true;fail();},15000);document.getElementById('signin-retry').addEventListener('click',function(){window.location.reload();});window.addEventListener('load', async function () {try {if(!window.Clerk)throw new Error('Unavailable');await Clerk.load({ui:{ClerkUI:window.__internal_ClerkUICtor}});if(timedOut)return;clearTimeout(deadline);if(${switchAccount ? "true" : "false"} && Clerk.session){target.hidden=true;document.getElementById('switch-account').hidden=false;document.getElementById('switch-signout').addEventListener('click',async function(){try{await Clerk.signOut({redirectUrl:${JSON.stringify("/sign-in?returnTo=" + encodeURIComponent(target))}})}catch{fail();}});}else{target.textContent='';Clerk.mountSignIn(target,${mountOptions});}} catch {clearTimeout(deadline);fail();}});})();</script>`;
  return accessDocument("Sign in", content, scripts);
}

export function renderCallbackNotice(title: string, message: string): string {
  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>${title} — gog-marketing</title>${FAVICON_LINK}<style>${PRODUCT_CSS}</style></head><body><main class="shell"><section class="card intro"><p class="eyebrow">gog-marketing</p><h1>${title}</h1><p>${message}</p><a class="button primary" href="/">Return to workspace</a></section></main><script>window.history.replaceState(null, '', window.location.pathname);</script></body></html>`;
}

/** Public help and privacy page in the shared public shell; plain language, no login (P02, N02). */
export function renderPublicAbout(): string {
  const faq: Array<[string, string]> = [
    [
      "What can gog-marketing see in my Google account?",
      "Only the services you approve when you connect an account, and then only the files and resources you choose. Approving a service does not turn on any assets or AI tools by itself.",
    ],
    [
      "Can the AI change my Google data?",
      "Only if you allow changes for that service and that exact tool. Each change is shown to you first and runs only after you approve it. Deleting or replacing data needs a separate permission.",
    ],
    [
      "Why did chat say it reached a limit?",
      "Chat uses shared free model capacity with per-minute and daily limits. Open “Model details and today’s usage” under the chat box to see your usage and when it resets. No paid model is ever used automatically.",
    ],
    [
      "How do I remove access?",
      "On Home, open the account’s ⋮ menu and choose Disconnect account. You can also revoke access at any time from your Google Account’s third-party connections page.",
    ],
    [
      "How do I delete a conversation?",
      "Open the conversation in chat and choose Delete. This removes its messages and pending approvals. It does not undo anything already done in Google.",
    ],
  ];
  return accessDocument(
    "Help & privacy",
    `<main id="main-content" tabindex="-1" class="access-main access-prose"><h1>Help &amp; privacy</h1><p class="access-lead">gog-marketing connects the Google accounts you choose to AI chat and tools, under permissions you control.</p><section id="help" aria-labelledby="help-heading"><h2 id="help-heading">Common questions</h2>${faq
      .map(
        ([question, answer]) =>
          `<details class="faq"><summary>${question}</summary><p>${answer}</p></details>`,
      )
      .join(
        "",
      )}</section><section aria-labelledby="privacy-heading"><h2 id="privacy-heading">What we access and who processes it</h2><ul><li><strong>Google sign-in stays with Google.</strong> Signing in to gog-marketing is separate from connecting a Google account.</li><li><strong>Credentials are encrypted.</strong> Google access tokens are stored encrypted and are never given to the AI model.</li><li><strong>AI processing.</strong> Chat messages, and Google results you ask for, are processed by OpenRouter and the model provider you select. Don’t share data in chat that you aren’t allowed to share with them.</li><li><strong>Your conversations.</strong> Each conversation keeps its own history. You can rename or delete it at any time.</li></ul></section><section aria-labelledby="status-heading"><h2 id="status-heading">Beta status</h2><p>gog-marketing is a private, invitation-only beta. Supported tools and usage are limited, and billing and long-term memory aren’t offered yet. Our formal privacy policy and Google’s app verification are still being finalised, so this page is a plain-language summary, not a legal policy.</p></section><p class="access-actions"><a class="button primary" href="/sign-in">Sign in</a><a href="/">Back to the product</a></p></main>`,
  );
}
