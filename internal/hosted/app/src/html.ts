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
export function securityHeaders(clerkDomain = ""): Record<string, string> {
  const clerkSources = clerkDomain ? [`https://${clerkDomain}`, `wss://${clerkDomain}`] : [];
  const clerkScriptSources = clerkDomain ? [`https://${clerkDomain}`] : [];
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline' ${clerkScriptSources.join(" ")} https://challenges.cloudflare.com https://*.protect.clerk.com`,
    "style-src 'self' 'unsafe-inline'",
    `img-src 'self' data: ${clerkScriptSources.join(" ")} https://img.clerk.com`,
    `font-src 'self' ${clerkScriptSources.join(" ")}`,
    `connect-src 'self' ${clerkSources.join(" ")} https://*.protect.clerk.com:*`,
    `frame-src ${clerkScriptSources.join(" ")} https://challenges.cloudflare.com https://*.protect.clerk.com`,
    "worker-src 'self' blob:",
    `form-action 'self' ${clerkScriptSources.join(" ")}`,
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
.account-row { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; padding: 0.5rem 0; border-bottom: 1px solid #30363d; }
.account-row:last-child { border-bottom: none; }
.account-name { font-size: 0.9rem; }
.account-status { color: #8b949e; font-size: 0.75rem; }
.account-actions { display: flex; gap: 0.4rem; }
.account-actions button {
  background: #21262d; color: #e6edf3; border: 1px solid #30363d; border-radius: 6px;
  padding: 0.3rem 0.6rem; font-size: 0.75rem; cursor: pointer;
}
.account-actions button:hover { border-color: #58a6ff; }
#connect-google {
  background: #1f6feb; color: #ffffff; border: none; border-radius: 6px;
  padding: 0.5rem 0.9rem; font-size: 0.85rem; cursor: pointer; margin-top: 0.75rem;
}
#connect-google:hover { background: #388bfd; }
#google-message { color: #8b949e; font-size: 0.8rem; margin-top: 0.5rem; min-height: 1rem; }
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
 * Render a static branded notice for terminal Clerk failures on the Google
 * callback route. The shell intentionally has NO Clerk SDK scripts: the SDK
 * must never load when authentication configuration is invalid, and a
 * terminal-failure page gains nothing from it. The renderer never receives
 * request-derived values — only fixed operator-defined titles and messages —
 * so OAuth code/state/nonce can never be echoed into the page. The inline
 * script strips the secret-bearing query parameters from the address bar
 * without navigating anywhere, which cannot create a redirect loop.
 */
export function renderCallbackNotice(title: string, message: string): string {
  const content = `
  <div class="brand">gog-marketing</div>
  <div class="sub">Google connection could not be completed.</div>
  <div class="card">
    <span class="tag">${escapeHtmlAttr(title)}</span>
    <p class="home-info">${escapeHtmlAttr(message)}</p>
    <div class="home-actions">
      <a href="/">Return to the workspace</a>
    </div>
  </div>
  <script>
    window.history.replaceState(null, '', '/oauth/google/callback');
  </script>`;
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>${escapeHtmlAttr(title)} — gog-marketing</title>
  <style>${CSS}</style>
</head>
<body>
  <div class="shell">${content}</div>
</body>
</html>`;
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
  <div class="card" style="margin-top: 1rem;">
    <span class="tag">Google accounts</span>
    <div id="google-accounts"></div>
    <button id="connect-google" type="button">Connect Google account</button>
    <div id="google-message"></div>
  </div>
  <script>
    function googleMessage(text) {
      var el = document.getElementById('google-message');
      if (el) el.textContent = text;
    }
    async function loadGoogleAccounts() {
      var target = document.getElementById('google-accounts');
      if (!target) return;
      try {
        var res = await fetch('/api/google/connections');
        if (!res.ok) { target.textContent = ''; return; }
        var data = await res.json();
        target.textContent = '';
        (data.connections || []).forEach(function (connection) {
          var row = document.createElement('div');
          row.className = 'account-row';
          var info = document.createElement('div');
          var name = document.createElement('div');
          name.className = 'account-name';
          name.textContent = connection.email || connection.displayName || 'Google account';
          var status = document.createElement('div');
          status.className = 'account-status';
          var discovery = connection.discovery || {};
          var discoveryText = discovery.status === 'ok' ? '' :
            discovery.status === 'empty' ? ' · no resources found' :
            discovery.status === 'unavailable' ? ' · discovery unavailable' :
            discovery.status === 'error' ? ' · discovery failed' : '';
          status.textContent = 'status: ' + connection.status + discoveryText;
          info.appendChild(name);
          info.appendChild(status);
          var actions = document.createElement('div');
          actions.className = 'account-actions';
          var refreshButton = document.createElement('button');
          refreshButton.type = 'button';
          refreshButton.textContent = 'Refresh';
          refreshButton.addEventListener('click', async function () {
            googleMessage('');
            var r = await fetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
            if (!r.ok) {
              var err = await r.json().catch(function () { return {}; });
              googleMessage(err.error || 'Refresh failed.');
              return;
            }
            await loadGoogleAccounts();
          });
          var disconnectButton = document.createElement('button');
          disconnectButton.type = 'button';
          disconnectButton.textContent = 'Disconnect';
          disconnectButton.addEventListener('click', async function () {
            googleMessage('');
            var r = await fetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/disconnect', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
            if (!r.ok) {
              var err = await r.json().catch(function () { return {}; });
              googleMessage(err.error || 'Disconnect failed.');
              return;
            }
            await loadGoogleAccounts();
          });
          var reconnectButton = document.createElement('button');
          reconnectButton.type = 'button';
          reconnectButton.textContent = 'Reconnect';
          reconnectButton.addEventListener('click', async function () {
            var response = await fetch('/api/google/connect', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ connectionId: connection.id }) });
            var data = await response.json().catch(function () { return {}; });
            if (response.ok && data.authorizationUrl) window.location.assign(data.authorizationUrl);
            else googleMessage(data.error || 'Could not reconnect Google account.');
          });
          actions.appendChild(reconnectButton);
          actions.appendChild(refreshButton);
          actions.appendChild(disconnectButton);
          row.appendChild(info);
          row.appendChild(actions);
          target.appendChild(row);
        });
      } catch {
        target.textContent = '';
      }
    }
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
      var connectButton = document.getElementById('connect-google');
      if (connectButton) {
        connectButton.addEventListener('click', async function () {
          googleMessage('');
          var res = await fetch('/api/google/connect', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
          var data = await res.json().catch(function () { return {}; });
          if (res.ok && data.authorizationUrl) {
            window.location.assign(data.authorizationUrl);
            return;
          }
          googleMessage(data.error || 'Could not start Google connection.');
        });
      }
      var outcome = new URL(window.location.href).searchParams.get('google');
      var messages = {
        connected: 'Google account connected.',
        consent_denied: 'Google consent was denied. Your previous access was not changed.',
        operator_config_missing: 'Google connection is not configured by the operator yet.',
        google_unavailable: 'Google is temporarily unreachable. Please try again.',
        google_api_error: 'Google could not complete the connection. Please try again.',
        identity_conflict: 'Choose the original Google account when reconnecting.',
        state_expired: 'The connection attempt expired. Please start again.',
        state_replayed: 'This connection attempt was already completed.',
        authorization_expired: 'The Google authorization attempt expired. Please start again.',
        needs_reconnect: 'This Google account needs to be reconnected.'
      };
      if (outcome) {
        googleMessage(messages[outcome] || 'The Google connection attempt could not be completed. Please start again.');
        window.history.replaceState(null, '', '/');
      }
      await loadGoogleAccounts();
    });
  </script>`;
  return renderShell("Home — gog-marketing", content, htmlConfig);
}
