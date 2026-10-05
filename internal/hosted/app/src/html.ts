/**
 * HTML shell rendering for the hosted Worker.
 *
 * Serves a thin Clerk-protected UI: sign-in/sign-up for unauthenticated
 * visitors, a minimal product home with a real Clerk UserButton and sign-out
 * for authenticated users. The UI only claims sign-in, sign-out, and product
 * home — no Google Connect, runner, control-plane, or MCP features are shown
 * or faked.
 */

export interface HtmlConfig {
  publishableKey: string;
  authorizedParties: string[];
  clerkDomain: string;
}

/** Derive the Clerk Frontend API domain from a publishable key. */
export function deriveClerkDomain(publishableKey: string): string {
  try {
    const parts = publishableKey.split("_");
    if (parts.length < 3 || !parts[2]) return "";
    const domain = atob(parts[2]).slice(0, -1);
    if (!isValidClerkDomain(domain)) return "";
    return domain;
  } catch {
    return "";
  }
}

function isValidClerkDomain(domain: string): boolean {
  return /^[a-z0-9][a-z0-9.-]*\.[a-z]{2,}$/.test(domain);
}

/** Escape a value for use inside a double-quoted HTML attribute. */
function escapeHtmlAttr(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/**
 * Baseline CSP follows Clerk's manual CSP guide, including bot/fraud hosts:
 * https://clerk.com/docs/guides/secure/best-practices/csp-headers
 * `unsafe-inline` supports the shell loader and Clerk runtime styles. The
 * protect-host port wildcard is required only for connect-src; no broad
 * all-origin or http(s)-scheme sources are allowed.
 */
export function securityHeaders(clerkDomain: string): Record<string, string> {
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline' https://${clerkDomain} https://challenges.cloudflare.com https://*.protect.clerk.com`,
    "style-src 'self' 'unsafe-inline'",
    `img-src 'self' data: https://${clerkDomain} https://img.clerk.com`,
    `font-src 'self' https://${clerkDomain}`,
    `connect-src 'self' https://${clerkDomain} wss://${clerkDomain} https://*.protect.clerk.com:*`,
    `frame-src https://${clerkDomain} https://challenges.cloudflare.com https://*.protect.clerk.com`,
    "worker-src 'self' blob:",
    `form-action 'self' https://${clerkDomain}`,
    "object-src 'none'",
    "base-uri 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
  return {
    "Content-Security-Policy": csp,
    "X-Content-Type-Options": "nosniff",
  };
}

const CSS = `
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  background: #0d1117;
  color: #e6edf3;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}
.shell { max-width: 420px; width: 100%; }
.brand { font-size: 1.75rem; font-weight: 700; margin-bottom: 0.5rem; color: #58a6ff; }
.sub { color: #8b949e; margin-bottom: 1.5rem; font-size: 0.95rem; }
.card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 1.5rem;
}
.tag {
  display: inline-block;
  background: #1f6feb33;
  color: #58a6ff;
  border-radius: 4px;
  padding: 0.15rem 0.5rem;
  font-size: 0.75rem;
  font-weight: 500;
  margin-bottom: 1rem;
}
.home-info { line-height: 1.7; }
.home-info strong { color: #e6edf3; }
.home-info .muted { color: #8b949e; font-size: 0.85rem; }
.home-actions { display: flex; align-items: center; gap: 0.75rem; margin-top: 1rem; }
#sign-out {
  background: #21262d;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 0.45rem 0.9rem;
  font-size: 0.85rem;
  cursor: pointer;
}
#sign-out:hover { border-color: #58a6ff; }
#user-button { min-height: 2.25rem; }
`;

function clerkScriptTags(htmlConfig: HtmlConfig): string {
  const pk = escapeHtmlAttr(htmlConfig.publishableKey);
  return `
  <script defer crossorigin="anonymous" type="text/javascript"
    src="https://${htmlConfig.clerkDomain}/npm/@clerk/ui@1/dist/ui.browser.js"></script>
  <script defer crossorigin="anonymous" type="text/javascript"
    data-clerk-publishable-key="${pk}"
    src="https://${htmlConfig.clerkDomain}/npm/@clerk/clerk-js@6/dist/clerk.browser.js"></script>`;
}

function renderShell(title: string, content: string, htmlConfig: HtmlConfig): string {
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>${title}</title>
  <style>${CSS}</style>
${clerkScriptTags(htmlConfig)}
</head>
<body>
  <div class="shell">${content}</div>
</body>
</html>`;
}

/** Render the sign-in / sign-up page for unauthenticated visitors. */
export function renderSignIn(htmlConfig: HtmlConfig): string {
  const content = `
  <div class="brand">gog-marketing</div>
  <div class="sub">Sign in to access your marketing workspace.</div>
  <div class="card">
    <span class="tag">Clerk authentication</span>
    <div id="sign-in"></div>
  </div>
  <script>
    window.addEventListener('load', async function () {
      await Clerk.load({ ui: { ClerkUI: window.__internal_ClerkUICtor } });
      var el = document.getElementById('sign-in');
      if (el) Clerk.mountSignIn(el, { fallbackRedirectUrl: '/' });
    });
  </script>`;
  return renderShell("Sign in — gog-marketing", content, htmlConfig);
}

/**
 * Render the protected product home for an authenticated user.
 * Loads the real Clerk SDK and exposes account management through the
 * documented Clerk `UserButton`, plus an explicit sign-out action that calls
 * `Clerk.signOut()` and reloads the server-rendered home route.
 */
export function renderHome(htmlConfig: HtmlConfig): string {
  const content = `
  <div class="brand">gog-marketing</div>
  <div class="sub">Your workspace is ready.</div>
  <div class="card">
    <div class="home-info">
      <span class="tag">Signed in</span>
      <p><strong>Workspace:</strong> ready</p>
    </div>
    <div class="home-actions">
      <button id="sign-out" type="button">Sign out</button>
      <div id="user-button"></div>
    </div>
  </div>
  <script>
    window.addEventListener('load', async function () {
      await Clerk.load({ ui: { ClerkUI: window.__internal_ClerkUICtor } });
      var userButton = document.getElementById('user-button');
      if (userButton) Clerk.mountUserButton(userButton);
      var signOutButton = document.getElementById('sign-out');
      if (signOutButton) {
        signOutButton.addEventListener('click', async function () {
          await Clerk.signOut({ redirectUrl: '/' });
        });
      }
    });
  </script>`;
  return renderShell("Home — gog-marketing", content, htmlConfig);
}
