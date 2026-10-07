/**
 * HTML shell rendering for the hosted Worker.
 *
 * Serves the Clerk-protected hosted product shell: sign-in/sign-up for
 * unauthenticated visitors and the marketer workspace for authenticated
 * users. It preserves the existing Go marketer UI
 * structure and safety language, while the MCP panel is explicitly marked as
 * not yet authenticated/working.
 */

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
:root {
  --surface-page: #f4f6f9;
  --surface-card: #ffffff;
  --surface-sunken: #eef1f5;
  --text: #1a2333;
  --text-muted: #5a6577;
  --text-faint: #627086;
  --border: #dde3ec;
  --border-strong: #c3ccda;
  --action: #245bd6;
  --action-hover: #1e4fbf;
  --action-soft: #e9effb;
  --status-ok: #176f30;
  --status-ok-soft: #e6f4ea;
  --status-warn: #855900;
  --status-warn-soft: #fdf3d7;
  --radius: 10px;
  --radius-l: 14px;
  --shadow: 0 1px 2px rgb(23 32 51 / .06), 0 4px 12px rgb(23 32 51 / .07);
  --focus-ring: 0 0 0 2px var(--surface-card), 0 0 0 4px var(--action);
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: "Avenir Next", "Segoe UI", system-ui, -apple-system, sans-serif;
  font-size: 15px;
  line-height: 1.5;
  color: var(--text);
  background: var(--surface-page);
  -webkit-font-smoothing: antialiased;
}
a { color: var(--action); text-decoration: none; }
h1 { font-size: 26px; font-weight: 650; letter-spacing: -.015em; margin: 0 0 8px; }
h2 { font-size: 18px; font-weight: 650; margin: 0 0 12px; }
h3 { font-size: 15.5px; font-weight: 600; margin: 0; }
p { margin: 0 0 12px; color: var(--text-muted); }
:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
.shell { max-width: 520px; margin: 48px auto; padding: 0 20px; }
.workspace { min-height: 100vh; background: var(--surface-page); color: var(--text); }
.shell-top {
  position: sticky; top: 0; z-index: 2;
  background: var(--surface-card); border-bottom: 1px solid var(--border);
  padding: 12px 24px; display: flex; align-items: center; gap: 16px;
}
.shell-brand { color: var(--text); font-weight: 700; font-size: 15px; }
.shell-nav { margin-left: auto; display: flex; align-items: center; gap: 16px; }
.shell-account { color: var(--text-muted); font-size: 13.5px; }
main { max-width: 1120px; margin: 32px auto; padding: 0 24px; }
main > * + * { margin-top: 24px; }
.eyebrow {
  margin: 0 0 4px; font-size: 12px; font-weight: 650; text-transform: uppercase;
  letter-spacing: .08em; color: var(--text-faint);
}
.intro { max-width: 660px; }
.tiles { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.tile { background: var(--surface-card); border: 1px solid var(--border); border-radius: var(--radius); padding: 16px 20px; }
.tile-label { color: var(--text-muted); font-size: 13px; margin: 0 0 2px; }
.tile-value { font-size: 24px; font-weight: 650; font-variant-numeric: tabular-nums; margin: 0; }
.tile small { color: var(--text-faint); font-size: 12.5px; }
.card { background: var(--surface-card); border: 1px solid var(--border); border-radius: var(--radius-l); padding: 20px; }
.card-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.card-actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
button {
  font: inherit; font-size: 14px; font-weight: 600; color: var(--text);
  background: var(--surface-card); border: 1px solid var(--border-strong);
  border-radius: 6px; padding: 8px 14px; min-height: 40px; cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
  transition: background 140ms ease, border-color 140ms ease;
}
button:hover { background: var(--surface-sunken); }
button.primary { background: var(--action); border-color: var(--action); color: #fff; }
button.primary:hover { background: var(--action-hover); }
button.attention { background: var(--status-warn-soft); border-color: #d9a441; color: var(--status-warn); }
button:disabled { cursor: default; opacity: .7; }
.stack { display: flex; flex-direction: column; }
.connect-help { font-size: 15.5px; }
.service-consent { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 16px; margin-bottom: 12px; }
.service-choice { display: inline-flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13.5px; }
.service-choice input { width: 18px; height: 18px; accent-color: var(--action); }
.connect-actions { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.account-card + .account-card { margin-top: 16px; }
.status {
  display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;
  background: var(--status-ok-soft); color: var(--status-ok); border-radius: 999px;
  padding: 3px 10px; font-size: 12.5px; font-weight: 600;
}
.status::before { content: ""; width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
.status.needs_attention, .status.unavailable { background: var(--status-warn-soft); color: var(--status-warn); }
.status.disconnected { background: var(--surface-sunken); color: var(--text-muted); }
.alert { border: 1px solid var(--border); border-left: 4px solid #d9a441; border-radius: var(--radius); padding: 16px; margin-top: 16px; }
.alert h3 { color: var(--status-warn); margin-bottom: 4px; }
.toast {
  display: flex; align-items: center; padding: 12px 16px; font-size: 14px;
  border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--surface-card); box-shadow: var(--shadow);
}
.toast.error { color: #c22731; border-left: 4px solid #c22731; }
.toast.info { color: var(--status-ok); border-left: 4px solid var(--status-ok); }
.picker { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 24px; align-items: start; }
.rail { position: sticky; top: 80px; display: grid; gap: 8px; }
.rail-group-label { margin: 8px 0 2px; color: var(--text-faint); font-size: 11.5px; font-weight: 650; text-transform: uppercase; letter-spacing: .08em; }
.rail-item {
  display: flex; justify-content: space-between; align-items: center; gap: 8px;
  background: var(--surface-card); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 10px 12px; color: var(--text); font-size: 13.5px; font-weight: 600;
}
.service-block { background: var(--surface-card); border: 1px solid var(--border); border-radius: var(--radius-l); padding: 16px 20px; margin-bottom: 16px; scroll-margin-top: 24px; }
.service-meta { margin: 0 0 12px; color: var(--text-muted); font-size: 13px; }
.rows { border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.row { display: flex; gap: 12px; align-items: flex-start; padding: 12px 16px; border-bottom: 1px solid var(--border); }
.row:last-child { border-bottom: 0; }
.row input[type=checkbox] { width: 18px; height: 18px; flex: none; margin-top: 3px; accent-color: var(--action); }
.row-label { flex: 1; min-width: 0; cursor: pointer; display: block; }
.row-label strong { display: block; font-size: 14.5px; overflow-wrap: anywhere; }
.row-label small { display: block; margin-top: 1px; color: var(--text-muted); font-size: 12.5px; }
.row-details { max-width: 280px; color: var(--text-faint); font-size: 12px; }
.row-details summary { color: var(--action); cursor: pointer; font-weight: 600; }
.savebar {
  position: sticky; bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px;
  background: var(--surface-card); border: 1px solid var(--border); border-radius: var(--radius);
  box-shadow: var(--shadow); padding: 12px 16px; margin-top: 16px;
}
.savebar p { margin: 0; font-size: 13.5px; }
.empty { text-align: center; padding: 48px 24px; }
.ai-panel .mcp-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.ai-panel code {
  flex: 1; min-width: 280px; padding: 10px 12px; border: 1px solid var(--border);
  border-radius: 6px; background: var(--surface-sunken); overflow-wrap: anywhere;
}
.setup-list { padding-left: 20px; color: var(--text-muted); }
.muted { color: var(--text-muted); }
#user-button { min-height: 40px; }
@media (max-width: 900px) {
  .picker { grid-template-columns: 1fr; }
  .rail { position: static; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .tiles, .service-consent { grid-template-columns: 1fr; }
}
@media (max-width: 600px) {
  .shell-top, main { padding-left: 16px; padding-right: 16px; }
  main { margin-top: 20px; }
  .shell-nav { width: 100%; margin-left: 0; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
  .shell-account { order: 4; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .card, .service-block, .tile { padding: 16px; }
  .card-head { flex-direction: column; }
  .card-actions { width: 100%; }
  .row { flex-wrap: wrap; }
  .savebar { bottom: 0; flex-wrap: wrap; }
  .savebar button { width: 100%; }
  .ai-panel code { min-width: 0; }
}
@media (prefers-reduced-motion: reduce) { * { transition: none !important; animation: none !important; } }
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

function renderShell(
  title: string,
  content: string,
  htmlConfig: HtmlConfig,
  shellClass = "shell",
): string {
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
  <div class="${shellClass}">${content}</div>
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
 Render the protected product home for an authenticated user.
 * The visual structure intentionally follows the existing Go marketer product:
 * an overview, account cards, a service rail, asset rows, and explicit safety
 * controls. The app is lightweight and uses no external framework.
 */
export function renderHome(htmlConfig: HtmlConfig): string {
  const canonicalMcpUrl = htmlConfig.mcpOrigin
    ? new URL("/mcp", htmlConfig.mcpOrigin).toString()
    : "";
  const content = `
  <div class="workspace">
    <header class="shell-top">
      <a class="shell-brand" href="/">gog-marketing</a>
      <nav class="shell-nav" aria-label="Primary">
        <span class="shell-account">Workspace</span>
        <button id="sign-out" type="button">Sign out</button>
        <div id="user-button"></div>
      </nav>
    </header>
    <main id="main-content" tabindex="-1">
      <div class="intro">
        <p class="eyebrow">Home</p>
        <h1>Your Google accounts</h1>
        <p>Your workspace is ready.</p>
        <p>Connect accounts with read-only consent, then choose exactly which assets your agents may use.</p>
      </div>
      <div id="app-message" class="toast" role="status" aria-live="polite" hidden><span></span></div>
      <div class="tiles" aria-label="Workspace summary">
        <div class="tile"><p class="tile-label">Google accounts</p><p class="tile-value" id="tile-accounts">—</p><small id="tile-connected">—</small></div>
        <div class="tile"><p class="tile-label">Assets selected</p><p class="tile-value" id="tile-selected">—</p><small id="tile-assets">—</small></div>
      </div>
      <section aria-label="Google accounts">
        <div id="google-accounts"></div>
      </section>
      <section class="stack">
        <h2>Connect a Google account</h2>
        <p class="connect-help">Select only what this account should use. Access is read-only and agents start with zero assets enabled.</p>
        <div id="connect-services" class="service-consent" aria-label="Optional read-only services"></div>
        <div class="connect-actions">
          <button id="connect-google" type="button" class="primary">Connect Google account</button>
          <span id="connect-status" class="muted" role="status" aria-live="polite"></span>
        </div>
      </section>
      <section id="asset-workspace" hidden aria-label="Assets and access">
        <div class="picker">
          <nav class="rail" id="asset-rail" aria-label="Google services"></nav>
          <div id="asset-groups"></div>
        <div class="savebar">
          <p>Saving applies only asset changes visible above. New discoveries stay disabled.</p>
          <button id="save-grants" type="button" class="primary">Save access</button>
        </div>
        </div>
      </section>
      <section class="card ai-panel" aria-labelledby="ai-panel-heading">
        <p class="eyebrow">Connect to your AI</p>
        <h2 id="ai-panel-heading">Codex remote MCP endpoint</h2>
        <p>This is the canonical public HTTPS address for your workspace. The endpoint is prepared, but authentication is not enabled yet; Codex will not connect successfully today.</p>
        <div class="mcp-row">
          <code id="mcp-url">${canonicalMcpUrl || "Workspace address is not configured."}</code>
          <button id="copy-mcp-url" type="button"${canonicalMcpUrl ? "" : " disabled"}>Copy URL</button>
        </div>
        <ol class="setup-list">
          <li>Copy the URL and keep it for your future Codex remote MCP configuration.</li>
          <li>When authentication is enabled, add the URL as a remote MCP server in Codex.</li>
          <li>Codex will then prompt you to complete browser OAuth. Do not paste tokens.</li>
        </ol>
        <p class="muted">Tenant identifiers, database IDs, provider IDs, and credentials are never shown here.</p>
      </section>
    </main>
  </div>
  <script>
    var SERVICE_CATEGORIES = {
      analytics: 'Marketing', googleads: 'Marketing', tagmanager: 'Marketing',
      searchconsole: 'Marketing', bigquery: 'Marketing',
      gmail: 'Workspace', calendar: 'Workspace', drive: 'Workspace'
    };
    var SERVICE_LABELS = {
      analytics: 'Google Analytics 4', googleads: 'Google Ads',
      tagmanager: 'Google Tag Manager', searchconsole: 'Search Console',
      bigquery: 'BigQuery', gmail: 'Gmail', calendar: 'Calendar', drive: 'Drive'
    };
    var data = { connections: [] };
    var selectedId = null;
    var originalChoices = new Map();
    var changedChoices = new Map();

    function el(id) { return document.getElementById(id); }
    function setText(id, value) { var target = el(id); if (target) target.textContent = value; }
    function message(text, kind) {
      var target = el('app-message');
      if (!target) return;
      target.className = 'toast' + (kind ? ' ' + kind : '');
      target.hidden = !text;
      target.firstElementChild.textContent = text;
    }
    function safeStatus(connection) {
      if (connection.status === 'needs_reconnect') return ['needs_attention', 'Needs reconnect'];
      if (connection.status === 'revoked') return ['unavailable', 'Revoked'];
      var discovery = connection.discovery || {};
      if (discovery.status === 'ok') return ['ok', 'Discovery ready'];
      if (discovery.status === 'empty') return ['disconnected', 'No resources found'];
      if (discovery.status === 'unavailable') return ['unavailable', 'Discovery unavailable'];
      if (discovery.status === 'error') return ['unavailable', 'Discovery failed'];
      return ['disconnected', 'Not checked'];
    }
    function category(service) { return SERVICE_CATEGORIES[service] || 'Google'; }
    function label(service) { return SERVICE_LABELS[service] || service; }
    function selectedAccount() { return data.connections.find(function (item) { return item.id === selectedId; }); }
    function selectedCount() {
      return data.connections.reduce(function (total, connection) {
        return total + (connection.resources || []).filter(function (resource) { return resource.enabled; }).length;
      }, 0);
    }
    function assetCount() {
      return data.connections.reduce(function (total, connection) { return total + (connection.resources || []).length; }, 0);
    }
    function updateTiles() {
      var connected = data.connections.filter(function (item) { return item.status === 'active'; }).length;
      setText('tile-accounts', String(data.connections.length));
      setText('tile-connected', connected + ' connected');
      setText('tile-selected', String(selectedCount()));
      setText('tile-assets', selectedCount() + ' of ' + assetCount() + ' discovered');
    }
    function renderAccounts() {
      var target = el('google-accounts');
      target.textContent = '';
      data.connections.forEach(function (connection) {
        var card = document.createElement('article');
        card.className = 'card account-card';
        var head = document.createElement('div');
        head.className = 'card-head';
        var info = document.createElement('div');
        var title = document.createElement('h3');
        title.textContent = connection.email || connection.displayName || 'Google account';
        var status = document.createElement('p');
        var statusData = safeStatus(connection);
        status.className = 'account-status';
        var badge = document.createElement('span');
        badge.className = 'status ' + statusData[0];
        badge.textContent = statusData[1];
        status.appendChild(badge);
        info.appendChild(title); info.appendChild(status);
        var actions = document.createElement('div');
        actions.className = 'card-actions';
        function action(buttonLabel, kind, listener) {
          var button = document.createElement('button');
          button.type = 'button'; button.textContent = buttonLabel;
          if (kind) button.className = kind;
          button.setAttribute('aria-busy', 'false');
          button.addEventListener('click', listener);
          actions.appendChild(button);
        }
        action('Manage access', '', function () { selectAccount(connection.id); });
        action('Reconnect', 'attention', function () { reconnect(connection); });
        action('Refresh', '', function () { refresh(connection); });
        action('Discover', '', function () { discover(connection); });
        action('Disconnect', '', function () { disconnect(connection); });
        head.appendChild(info); head.appendChild(actions); card.appendChild(head);
        if (connection.status !== 'active') {
          var alert = document.createElement('div');
          alert.className = 'alert attention';
          var h = document.createElement('h3'); h.textContent = 'Google needs your attention';
          var p = document.createElement('p'); p.textContent = 'Reconnect this account. Existing asset choices are preserved.';
          alert.appendChild(h); alert.appendChild(p); card.appendChild(alert);
        }
        target.appendChild(card);
      });
      updateTiles();
    }
    function serviceGroups(connection) {
      var groups = new Map();
      (connection.resources || []).forEach(function (resource) {
        var key = resource.service;
        if (!groups.has(key)) groups.set(key, { service: key, label: resource.serviceLabel || label(key), resources: [] });
        groups.get(key).resources.push(resource);
      });
      return Array.from(groups.values()).sort(function (a, b) { return a.label.localeCompare(b.label); });
    }
    function renderAssets() {
      var connection = selectedAccount();
      var workspace = el('asset-workspace');
      if (!connection) { workspace.hidden = true; return; }
      workspace.hidden = false;
      originalChoices.clear(); changedChoices.clear();
      var rail = el('asset-rail'); rail.textContent = '';
      var groups = serviceGroups(connection);
      var marketing = groups.filter(function (group) { return category(group.service) === 'Marketing'; });
      var workspaceGroups = groups.filter(function (group) { return category(group.service) === 'Workspace'; });
      var renderRailGroup = function (groupName, items) {
        if (!items.length) return;
        var heading = document.createElement('p'); heading.className = 'rail-group-label'; heading.textContent = groupName; rail.appendChild(heading);
        items.forEach(function (group) {
          var enabled = group.resources.filter(function (resource) { return resource.enabled; }).length;
          var item = document.createElement('a');
          item.className = 'rail-item'; item.href = '#rail-' + connection.id + '-' + group.service;
          var text = document.createElement('span'); text.textContent = group.label;
          var status = document.createElement('span'); status.className = 'status' + (enabled ? '' : ' disconnected'); status.textContent = enabled ? enabled + ' selected' : 'disabled';
          item.appendChild(text); item.appendChild(status); rail.appendChild(item);
        });
      };
      renderRailGroup('Marketing', marketing); renderRailGroup('Workspace', workspaceGroups);
      var target = el('asset-groups'); target.textContent = '';
      groups.forEach(function (group) {
        var block = document.createElement('section'); block.className = 'service-block'; block.id = 'rail-' + connection.id + '-' + group.service;
        var head = document.createElement('div'); head.className = 'card-head';
        var heading = document.createElement('h3'); heading.textContent = group.label;
        var meta = document.createElement('p'); meta.className = 'service-meta'; meta.textContent = group.resources.length + ' assets · ' + group.resources.filter(function (resource) { return resource.enabled; }).length + ' selected';
        head.appendChild(heading); head.appendChild(meta); block.appendChild(head);
        var rows = document.createElement('div'); rows.className = 'rows';
        group.resources.forEach(function (resource) {
          originalChoices.set(resource, resource.enabled);
          var row = document.createElement('div'); row.className = 'row';
          var input = document.createElement('input');
          input.type = 'checkbox'; input.checked = resource.enabled;
          var rowId = 'asset-' + connection.id + '-' + resource.service + '-' + resource.resourceId;
          input.id = rowId;
          var choice = { connectionId: connection.id, service: resource.service, resourceId: resource.resourceId };
          input.setAttribute('aria-label', (resource.displayName || resource.resourceId) + ' access');
          input.addEventListener('change', function () {
            var original = originalChoices.get(resource);
            if (input.checked === original) changedChoices.delete(choice); else changedChoices.set(choice, input.checked);
          });
          var box = document.createElement('label'); box.className = 'row-label'; box.setAttribute('for', rowId);
          var strong = document.createElement('strong'); strong.textContent = resource.displayName || resource.resourceId;
          var small = document.createElement('small'); small.textContent = resource.resourceType + (resource.parent ? ' · ' + resource.parent : '');
          box.appendChild(strong); box.appendChild(small);
          var details = document.createElement('details'); details.className = 'row-details';
          var summary = document.createElement('summary'); summary.textContent = 'Asset details';
          var code = document.createElement('code'); code.textContent = resource.resourceId;
          details.appendChild(summary); details.appendChild(code);
          row.appendChild(input); row.appendChild(box); row.appendChild(details); rows.appendChild(row);
        });
        block.appendChild(rows); target.appendChild(block);
      });
      if (!groups.length) {
        var empty = document.createElement('div'); empty.className = 'card empty';
        var heading = document.createElement('h2'); heading.textContent = 'No assets found yet';
        var copy = document.createElement('p');
        copy.textContent = discoveryText(connection) + ' Existing choices are unchanged.';
        empty.appendChild(heading); empty.appendChild(copy); target.appendChild(empty);
      }
    }
    function discoveryText(connection) {
      var discovery = connection.discovery || {};
      if (discovery.status === 'unavailable') return 'Discovery is temporarily unavailable.';
      if (discovery.status === 'error') return 'Discovery failed, so no new assets were selected.';
      if (discovery.status === 'empty') return 'The successful check found no assets.';
      return 'Run discovery to check this account.';
    }
    function selectAccount(id) {
      selectedId = id;
      renderAccounts(); renderAssets();
      el('asset-workspace').scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
    function jsonFetch(url, body) {
      return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : '{}' });
    }
    async function loadResources() {
      try {
        var response = await fetch('/api/google/resources');
        if (!response.ok) throw new Error();
        data = await response.json();
        if (!data.connections || !data.connections.length) { data.connections = []; selectedId = null; }
        else if (!selectedId || !data.connections.some(function (item) { return item.id === selectedId; })) selectedId = data.connections[0].id;
        renderAccounts(); renderAssets();
        return true;
      } catch { message('Could not load your Google assets. Try again.', 'error'); return false; }
    }
    async function guardedAction(actionLabel, connection, request) {
      message('');
      try {
        var response = await request();
        if (!response.ok) {
          var failure = await response.json().catch(function () { return {}; });
          throw new Error(failure.error || actionLabel + ' failed.');
        }
        var reloaded = await loadResources();
        if (!reloaded) return;
        message(actionLabel + ' complete.', 'info');
      } catch (error) { message(error.message || actionLabel + ' failed.', 'error'); }
    }
    function reconnect(connection) { startConnection({ connectionId: connection.id, services: selectedServices() }); }
    function refresh(connection) { guardedAction('Refresh', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/refresh'); }); }
    function discover(connection) { guardedAction('Discovery', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/discover'); }); }
    function disconnect(connection) {
      var confirmed = window.confirm('Disconnect ' + (connection.email || connection.displayName || 'this Google account') + '? Agents will lose access.');
      if (!confirmed) return;
      guardedAction('Disconnect', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/disconnect'); });
    }
    async function save() {
      var connection = selectedAccount(); if (!connection) return;
      var grants = Array.from(changedChoices.entries()).map(function (entry) {
        return { service: entry[0].service, resourceId: entry[0].resourceId, enabled: entry[1] };
      });
      if (!grants.length) { message('No changes to save.', 'info'); return; }
      await guardedAction('Access saved', connection, function () {
        return jsonFetch('/api/google/grants', { connectionId: connection.id, grants: grants });
      });
    }
    function renderConsentChoices() {
      var target = el('connect-services'); target.textContent = '';
      Object.keys(SERVICE_LABELS).forEach(function (service) {
        var wrapper = document.createElement('label'); wrapper.className = 'service-choice';
        var input = document.createElement('input'); input.type = 'checkbox'; input.value = service;
        wrapper.appendChild(input); wrapper.append(document.createTextNode(SERVICE_LABELS[service] + ' · read-only'));
        target.appendChild(wrapper);
      });
    }
    function selectedServices() {
      return Array.from(el('connect-services').querySelectorAll('input:checked')).map(function (input) { return input.value; });
    }
    async function startConnection(body) {
      var status = el('connect-status'); status.textContent = '';
      try {
        var response = await jsonFetch('/api/google/connect', body);
        var result = await response.json().catch(function () { return {}; });
        if (!response.ok || !result.authorizationUrl) throw new Error(result.error || 'Could not start Google connection.');
        window.location.assign(result.authorizationUrl);
      } catch (error) { status.textContent = error.message || 'Could not start Google connection.'; }
    }
    window.addEventListener('load', async function () {
      await Clerk.load({ ui: { ClerkUI: window.__internal_ClerkUICtor } });
      var userButton = el('user-button');
      if (userButton) Clerk.mountUserButton(userButton);
      var signOutButton = el('sign-out');
      if (signOutButton) signOutButton.addEventListener('click', function () { return Clerk.signOut({ redirectUrl: '/' }); });
      el('connect-google').addEventListener('click', function () { return startConnection({ services: selectedServices() }); });
      el('save-grants').addEventListener('click', function () { return save(); });
      var copyButton = el('copy-mcp-url');
      if (copyButton) copyButton.addEventListener('click', function () {
        navigator.clipboard.writeText(el('mcp-url').textContent);
        copyButton.textContent = 'Copied';
        setTimeout(function () { copyButton.textContent = 'Copy URL'; }, 1800);
      });
      var outcome = new URL(window.location.href).searchParams.get('google');
      var messages = { connected: 'Google account connected.', consent_denied: 'Google consent was denied. Your previous access was not changed.', operator_config_missing: 'Google connection is not configured by the operator yet.', google_unavailable: 'Google is temporarily unreachable. Please try again.', google_api_error: 'Google could not complete the connection. Please try again.', identity_conflict: 'Choose the original Google account when reconnecting.', state_expired: 'The connection attempt expired. Please start again.', needs_reconnect: 'This Google account needs to be reconnected.' };
      if (outcome) { message(messages[outcome] || 'The Google connection attempt could not be completed.'); window.history.replaceState(null, '', '/'); }
      renderConsentChoices();
      await loadResources();
    });
  </script>`;
  return renderShell("Home — gog-marketing", content, htmlConfig, "workspace");
}
