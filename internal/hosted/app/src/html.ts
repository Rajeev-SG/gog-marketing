import { PRODUCT_CSS } from "./product-css.generated.js";
import { HOSTED_SERVICES } from "./scopes.js";

/**
 * Consent-form slice: the custodian-priority services only, grouped for the
 * connect panel. Deferred services (contacts, YouTube, …) stay in
 * HOSTED_SERVICES for the allowlist and existing encrypted grants, but are not
 * offered in the connect-service consent UI.
 */
const CONSENT_GROUPS: ReadonlyArray<{ legend: string; services: string[] }> = [
  {
    legend: "Workspace",
    services: [
      "calendar",
      "drive",
      "drivelabels",
      "docs",
      "slides",
      "tasks",
      "sheets",
      "forms",
      "appscript",
    ],
  },
  { legend: "Marketing", services: ["analytics", "searchconsole", "bigquery", "tagmanager"] },
];

export function consentServiceIds(): string[] {
  return CONSENT_GROUPS.flatMap((group) => group.services);
}

const PRIORITY_CONSENT_SERVICES = new Set(consentServiceIds());

/** Catalog services intentionally omitted from the consent form this release. */
export function deferredConsentServiceIds(): string[] {
  return Object.keys(HOSTED_SERVICES)
    .filter((service) => !PRIORITY_CONSENT_SERVICES.has(service))
    .sort();
}

for (const service of consentServiceIds()) {
  if (!Object.hasOwn(HOSTED_SERVICES, service)) {
    throw new Error("consent service is not in the hosted catalog: " + service);
  }
}

/** One consent row per service: an associated read checkbox plus a separate write control. */
function serviceChoiceRow(service: string): string {
  const info = HOSTED_SERVICES[service];
  const name = escapeHtmlAttr(service);
  const read =
    '<label class="service-choice-read" for="consent-read-' +
    name +
    '"><input id="consent-read-' +
    name +
    '" type="checkbox" value="' +
    name +
    '" data-permission="read"><span class="service-choice-name">' +
    escapeHtmlAttr(info.label) +
    "</span></label>";
  const control = info.writeScopes.length
    ? '<label class="service-choice-write" for="consent-write-' +
      name +
      '"><input id="consent-write-' +
      name +
      '" type="checkbox" value="' +
      name +
      '" data-permission="write" disabled><span>Writes</span></label>'
    : '<span class="service-choice-note" id="consent-note-' + name + '">Read only</span>';
  return '<div class="service-choice" data-service="' + name + '">' + read + control + "</div>";
}

/** Grouped consent markup: Workspace and Marketing fieldsets, one card per service. */
export function consentGroupsMarkup(): string {
  return CONSENT_GROUPS.map(
    (group) =>
      '<fieldset class="service-consent-group" data-consent-group="' +
      group.legend +
      '"><legend>' +
      group.legend +
      '</legend><div class="service-consent-list">' +
      group.services.map(serviceChoiceRow).join("") +
      "</div></fieldset>",
  ).join("");
}

function consentToolbarMarkup(): string {
  return '<div class="connect-consent-toolbar"><button id="connect-select-all" type="button">Select all services</button><button id="connect-clear-services" type="button">Clear selection</button></div>';
}

function consentSummaryMarkup(): string {
  return (
    '<p class="connect-consent-summary" id="connect-consent-summary">' +
    consentServiceIds().length +
    " service choices are shown. Read and write consent controls Google data access only—it does not enable any assets or tool permissions.</p>"
  );
}

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

function icon(name: string): string {
  const paths: Record<string, string> = {
    home: '<path d="M3 10.8 12 3l9 7.8"/><path d="M5 9.8V21h14V9.8"/><path d="M9 21v-7h6v7"/>',
    data: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>',
    activity: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/>',
    settings:
      '<circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1l2-1.5-2-3.4-2.4 1a8 8 0 0 0-1.7-1L14.5 3h-5l-.4 2.6a8 8 0 0 0-1.7 1l-2.4-1-2 3.4L5 11a7 7 0 0 0 0 2l-2 1.5 2 3.4 2.4-1a8 8 0 0 0 1.7 1l.4 2.6h5l.4-2.6a8 8 0 0 0 1.7-1l2.4 1 2-3.4-2-1.5c.1-.3.1-.7.1-1Z"/>',
    shield: '<path d="M12 22s8-3.5 8-10V5l-8-3-8 3v7c0 6.5 8 10 8 10Z"/><path d="m9 12 2 2 4-5"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
    help: '<circle cx="12" cy="12" r="9"/><path d="M9.7 9a2.5 2.5 0 1 1 3.2 2.4c-.9.3-.9 1.1-.9 1.6"/><path d="M12 17h.01"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
    sparkles:
      '<path d="m12 3 1.4 4.1L17.5 8.5l-4.1 1.4L12 14l-1.4-4.1-4.1-1.4 4.1-1.4L12 3Z"/><path d="m18 14 .8 2.2L21 17l-2.2.8L18 20l-.8-2.2L15 17l2.2-.8L18 14Z"/>',
  };
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] ?? ""}</svg>`;
}

function clerkScriptTags(htmlConfig: HtmlConfig): string {
  const pk = escapeHtmlAttr(htmlConfig.publishableKey);
  return `<script defer crossorigin="anonymous" type="text/javascript" src="https://${htmlConfig.clerkDomain}/npm/@clerk/ui@1/dist/ui.browser.js"></script>
  <script defer crossorigin="anonymous" type="text/javascript" data-clerk-publishable-key="${pk}" src="https://${htmlConfig.clerkDomain}/npm/@clerk/clerk-js@6/dist/clerk.browser.js"></script>`;
}

function documentShell(title: string, content: string, htmlConfig: HtmlConfig): string {
  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>${title}</title><style>${PRODUCT_CSS}</style>${clerkScriptTags(htmlConfig)}</head><body>${content}</body></html>`;
}

function navItem(href: string, label: string, iconName: string, active = false): string {
  return `<a href="${href}"${active ? ' aria-current="page"' : ""}>${icon(iconName)}<span>${label}</span></a>`;
}

function appShell(content: string, htmlConfig: HtmlConfig): string {
  return documentShell(
    "Home — gog-marketing",
    `<a class="skip-link" href="#main-content">Skip to content</a><div class="app-shell" id="app-shell">
    <aside class="sidebar" id="app-sidebar" aria-label="Application navigation"><a class="sidebar-brand" href="/"><span class="brand-mark">g</span><span>gog-marketing</span></a><div class="sidebar-rule"></div><nav class="side-nav">${navItem("#home", "Home", "home", true)}${navItem("#google-data", "Google data", "data")}${navItem("#activity", "Activity", "activity")}${navItem("#settings", "Settings", "settings")}</nav><div class="sidebar-spacer"></div><div class="trust-card">${icon("shield")}<strong>Secure by design</strong><p>Least-privilege access to the Google data you choose.</p></div><div class="sidebar-user"><span class="avatar">G</span><span>Google access workspace</span></div></aside>
    <div class="app-frame"><header class="topbar"><button class="icon-button nav-toggle" id="nav-toggle" type="button" aria-label="Open navigation" aria-controls="app-sidebar" aria-expanded="false">☰</button><label class="topbar-search">${icon("search")}<input id="global-search" type="search" placeholder="Search" aria-label="Search workspace"></label><div class="topbar-actions"><a class="button secondary-action" href="#settings">${icon("help")} Help</a><button id="sign-out" type="button">Sign out</button><div id="user-button"></div></div></header><main class="app-main" id="main-content" tabindex="-1">${content}</main><footer class="app-footer"><span>gog-marketing · Secure access layer for Google marketing data</span><span>Privacy · Terms</span></footer></div>
    <nav class="bottom-nav" aria-label="Mobile navigation">${navItem("#home", "Home", "home", true)}${navItem("#google-data", "Google data", "data")}${navItem("#activity", "Activity", "activity")}${navItem("#settings", "Settings", "settings")}</nav></div>`,
    htmlConfig,
  );
}

export function renderSignIn(htmlConfig: HtmlConfig): string {
  return documentShell(
    "Sign in — gog-marketing",
    `<main class="shell" id="main-content" tabindex="-1"><section class="card intro sign-in-card"><p class="eyebrow">Get started</p><h1>Connect your Google marketing data</h1><p>Control exactly which Google accounts, services and assets your AI agents can access.</p><div id="sign-in"><p>Clerk authentication is loading…</p></div><p class="faint">Your credentials stay with Google. gog-marketing stores only the access you explicitly approve.</p></section></main><script>window.addEventListener('load', async function () { await Clerk.load({ ui: { ClerkUI: window.__internal_ClerkUICtor } }); var target = document.getElementById('sign-in'); if (target) Clerk.mountSignIn(target, { fallbackRedirectUrl: '/' }); });</script>`,
    htmlConfig,
  );
}

export function renderCallbackNotice(title: string, message: string): string {
  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>${title} — gog-marketing</title><style>${PRODUCT_CSS}</style></head><body><main class="shell"><section class="card intro"><p class="eyebrow">gog-marketing</p><h1>${title}</h1><p>${message}</p><a class="button primary" href="/">Return to workspace</a></section></main><script>window.history.replaceState(null, '', window.location.pathname);</script></body></html>`;
}

export function renderHome(htmlConfig: HtmlConfig): string {
  const canonicalMcpUrl = htmlConfig.mcpOrigin
    ? new URL("/mcp", htmlConfig.mcpOrigin).toString()
    : "";
  const serviceData = JSON.stringify(HOSTED_SERVICES).replaceAll("<", "\\u003c");
  const content = `<div id="app-message" class="toast" role="status" aria-live="polite" hidden><span></span><button class="toast-close" type="button" aria-label="Dismiss notification">×</button></div>
    <section class="state-banner" id="state-banner" hidden aria-live="polite"></section>
    <section class="panel completion" id="completion-state" hidden aria-labelledby="completion-heading"><div class="completion-progress" aria-label="Connection progress"><div class="progress-step done"><span class="progress-dot">${icon("check")}</span><span>1 Connect Google</span></div><div class="progress-step" id="completion-step-data"><span class="progress-dot" id="completion-step-data-dot">2</span><span>2 Choose data</span></div><div class="progress-step" id="completion-step-ready"><span class="progress-dot">3</span><strong>Ready</strong></div></div><div class="completion-body"><span class="completion-check" id="completion-check">${icon("check")}</span><h2 id="completion-heading">Google connected — choose your data</h2><p id="completion-description">No assets are available to connected AI tools until you make a selection.</p><div class="completion-metrics"><div class="completion-metric"><strong id="completion-accounts">0</strong><span>accounts connected</span></div><div class="completion-metric"><strong id="completion-services">0</strong><span>services enabled</span></div><div class="completion-metric"><strong id="completion-assets">0</strong><span>assets enabled</span></div></div><button class="button primary" id="completion-continue" type="button">Choose data →</button></div></section>
    <header class="page-header" id="home"><div><p class="eyebrow">Home</p><h1>Your Google marketing data</h1><p>Secure access and permissions for AI agents and tools.</p></div><div class="page-actions"><button class="button secondary-action" id="view-assets" type="button">${icon("data")} Manage data</button><button class="button primary" id="add-connection" type="button">${icon("plus")} Add Google account</button></div></header>
    <section class="panel loading-panel" id="loading-state" aria-live="polite"><div class="loading-heading"><span class="spinner" aria-hidden="true"></span><div><h2>Finding your Google marketing assets…</h2><p>Checking connected accounts and the access you already approved.</p></div></div><div class="skeleton-row"><span class="skeleton"></span><span class="skeleton"></span><span class="skeleton"></span></div><div class="skeleton-row"><span class="skeleton"></span><span class="skeleton"></span><span class="skeleton"></span></div><div class="skeleton-row"><span class="skeleton"></span><span class="skeleton"></span><span class="skeleton"></span></div></section>
    <section class="panel loading-panel" id="load-error" hidden><h2>We couldn’t load your Google access</h2><p>Your existing permissions have not changed.</p><button class="button primary" id="retry-load" type="button">Try again</button></section>
    <div id="workspace-content" hidden><section class="metric-strip" aria-label="Workspace summary"><div class="metric"><span class="metric-icon">${icon("data")}</span><div><strong id="tile-accounts">0</strong><span id="tile-connected">0 connected</span></div></div><div class="metric"><span class="metric-icon">${icon("shield")}</span><div><strong id="tile-selected">0</strong><span id="tile-assets">0 of 0 assets enabled</span></div></div><div class="metric"><span class="metric-icon">${icon("sparkles")}</span><div><strong id="tile-services">0</strong><span>services connected</span></div></div><div class="metric"><span class="metric-icon">${icon("check")}</span><div><strong id="tile-health">All healthy</strong><span id="tile-health-detail">No issues detected</span></div></div></section>
    <section class="panel" aria-labelledby="services-heading"><div class="panel-head"><div><h2 id="services-heading">Google services</h2><p>Connection status and enabled assets</p></div><button class="panel-link" id="manage-services" type="button">Manage access →</button></div><div class="service-table" id="service-overview-rows"></div></section>
    <div class="overview-grid"><section class="panel" aria-labelledby="accounts-heading"><div class="panel-head"><div><h2 id="accounts-heading">Connected Google accounts</h2><p>Accounts you’ve authorized</p></div><button class="panel-link" id="manage-accounts" type="button">Manage →</button></div><div class="account-list" id="account-overview"></div></section><section class="panel" id="activity" aria-labelledby="access-heading"><div class="panel-head"><div><h2 id="access-heading">Access model</h2><p>Least-privilege controls for connected tools</p></div></div><div class="quiet-list"><div class="quiet-item">${icon("shield")}<div><strong>Selected assets only</strong><p>Agents can access only the resources you explicitly enable.</p></div></div><div class="quiet-item">${icon("lock")}<div><strong>Read access by default</strong><p>Writes require separate service opt-in and tool permission.</p></div></div><div class="quiet-item">${icon("activity")}<div><strong>Independent accounts</strong><p>Credentials and grants remain isolated by account and tenant.</p></div></div></div></section></div>
    <section class="panel" id="google-data" aria-labelledby="google-data-heading"><div class="panel-head"><div><h2 id="google-data-heading">Google accounts</h2><p>Manage access for each connected account.</p></div></div><div id="google-accounts" style="padding:0 20px 20px"></div></section>
    <section class="panel" id="asset-workspace" hidden aria-labelledby="asset-heading"><div class="panel-head"><div><p class="eyebrow">Google data</p><h2 id="asset-heading">Manage asset access</h2><p>Choose the existing Google assets your AI agents and tools can access. This controls access, not reporting.</p></div><button class="button" id="pick-files" type="button">Choose Google files</button></div><div id="picker-status" role="status" aria-live="polite"></div><div class="asset-frame"><div class="asset-toolbar"><div class="searchbar"><input id="asset-search" type="search" placeholder="Search assets…" aria-label="Search assets"><button id="clear-search" type="button">Clear</button></div><div class="asset-summary"><strong id="asset-selected-count">0 selected</strong><span id="asset-health">● All services healthy</span></div></div><div class="asset-picker"><nav class="rail" id="asset-rail" aria-label="Google services"></nav><div class="asset-content" id="asset-groups"></div></div><div class="savebar"><p>New discoveries stay disabled until you choose them.</p><button id="save-grants" type="button" class="primary">Save changes</button></div></div></section>
    <section class="panel" id="tool-workspace" hidden aria-labelledby="tool-heading"><div class="panel-head"><div><h2 id="tool-heading">AI tool permissions</h2><p>Select exactly which tools this Google account may run. OAuth consent and asset selection do not enable writes.</p></div></div><div class="connect-panel"><div class="tool-list" id="tool-choices" aria-label="Tools for the selected Google account"></div><button id="save-tools" type="button" class="primary">Save tool permissions</button> <span id="tool-status" role="status" aria-live="polite"></span></div></section>
    <section class="panel connect-panel" id="connect-panel" aria-labelledby="connect-heading"><div class="connect-layout"><div><p class="eyebrow">Add access</p><h2 id="connect-heading">Connect a Google account</h2><p class="connect-help">Select only what this account should use. Writes require a separate opt-in per service; agents start with zero assets enabled.</p>${consentToolbarMarkup()}${consentSummaryMarkup()}<div id="connect-services" class="service-consent" aria-label="Google service read and write consent">${consentGroupsMarkup()}</div><div class="connect-actions"><button id="connect-google" type="button" class="primary">${icon("plus")}<span id="connect-google-label">Connect Google account</span></button><span id="connect-target" class="muted" role="status" aria-live="polite" hidden></span><span id="connect-status" class="muted" role="status" aria-live="polite"></span></div></div><aside class="safety-note"><strong>Least privilege by default</strong><p>Scope eligibility does not guarantee account or hosted-tool availability. Explicit tool and resource grants remain required.</p></aside></div></section>
    <section class="panel connect-panel" id="settings" aria-labelledby="mcp-heading"><p class="eyebrow">Connect to your AI</p><h2 id="mcp-heading">Codex remote MCP endpoint</h2>${canonicalMcpUrl ? `<p>Add this HTTPS URL as a remote MCP server in Codex. Complete the Clerk browser sign-in, then execute only the tools and assets enabled for your Google account.</p><div class="ai-panel"><code id="mcp-url">${canonicalMcpUrl}</code><button id="copy-mcp-url" type="button">Copy URL</button></div>` : `<p>Workspace address is not configured.</p><div class="ai-panel"><code>Remote MCP address unavailable</code><button id="copy-mcp-url" type="button" disabled>Copy URL</button></div>`}<p class="faint">Expanded service coverage is still being verified; this is not a public-beta readiness claim.</p></section></div>
  <script>
    var SERVICE_INFO = ${serviceData};
    var SERVICE_CATEGORIES = Object.fromEntries(Object.keys(SERVICE_INFO).map(function (name) { return [name, ['analytics','tagmanager','searchconsole','bigquery','adsense','youtube'].includes(name) ? 'Marketing' : 'Workspace']; }));
    var SERVICE_LABELS = Object.fromEntries(Object.entries(SERVICE_INFO).map(function (entry) { return [entry[0], entry[1].label]; }));
    var data = { connections: [] }; var selectedId = null; var activeService = ''; var assetDrafts = new Map();
    function el(id) { return document.getElementById(id); }
    function setText(id, value) { var target = el(id); if (target) target.textContent = value; }
    function message(text, kind) { var target = el('app-message'); if (!target) return; target.className = 'toast' + (kind ? ' ' + kind : ''); target.hidden = !text; target.firstElementChild.textContent = text; }
    function safeStatus(connection) {
      if (connection.status === 'needs_reconnect') return ['needs_attention', 'Needs attention'];
      if (connection.status === 'revoked') return ['unavailable', 'Access paused'];
      var discovery = connection.discovery || {};
      if (discovery.status === 'error' || discovery.status === 'unavailable') return ['unavailable', 'Check delayed'];
      if (discovery.status === 'empty') return ['disconnected', 'Ready'];
      return ['ok', 'Healthy'];
    }
    function category(service) { return SERVICE_CATEGORIES[service] || 'Google'; }
    function label(service) { return SERVICE_LABELS[service] || service; }
    function serviceKey(serviceLabel) { return Object.keys(SERVICE_LABELS).find(function (key) { return SERVICE_LABELS[key] === serviceLabel; }) || ''; }
    function serviceMark(service, serviceLabel) {
      var marks = { analytics: 'GA', tagmanager: 'GTM', searchconsole: 'SC', bigquery: 'BQ', drive: 'D', docs: 'Doc', sheets: 'Sh', slides: 'Sl', calendar: 'Cal', chat: 'Chat', forms: 'Form', youtube: 'YT' };
      var key = service || serviceKey(serviceLabel); return '<span class="service-mark ' + (key || 'default') + '" aria-hidden="true">' + (marks[key] || (serviceLabel || 'G').slice(0, 2)) + '</span>';
    }
    function resourceKey(resource) { return resource.service + '\u0000' + resource.resourceId; }
    function draftFor(connection) {
      if (!assetDrafts.has(connection.id)) {
        var draft = new Map(); (connection.resources || []).forEach(function (resource) { draft.set(resourceKey(resource), resource.enabled); }); assetDrafts.set(connection.id, draft);
      }
      var stored = assetDrafts.get(connection.id); (connection.resources || []).forEach(function (resource) { if (!stored.has(resourceKey(resource))) stored.set(resourceKey(resource), resource.enabled); }); return stored;
    }
    function selectedAccount() { return data.connections.find(function (item) { return item.id === selectedId; }); }
    function selectedCountFor(connection) { return (connection.resources || []).filter(function (resource) { return resource.enabled; }).length; }
    function selectedCount() { return data.connections.reduce(function (total, connection) { return total + (connection.resources || []).filter(function (resource) { return resource.enabled; }).length; }, 0); }
    function assetCount() { return data.connections.reduce(function (total, connection) { return total + (connection.resources || []).length; }, 0); }
    function serviceCount() {
      var keys = new Set(); data.connections.forEach(function (connection) { (connection.resources || []).forEach(function (resource) { keys.add(resource.service); }); (connection.services || []).forEach(function (name) { var key = serviceKey(name); if (key) keys.add(key); }); }); return keys.size;
    }
    function updateTiles() {
      var connected = data.connections.filter(function (item) { return item.status === 'active'; }).length; var needsAttention = data.connections.some(function (item) { return item.status !== 'active'; }); var delayed = data.connections.some(function (item) { return item.discovery.status === 'error' || item.discovery.status === 'unavailable'; });
      var health = needsAttention ? 'Needs attention' : delayed ? 'Some checks delayed' : 'All healthy'; var detail = needsAttention ? 'Review the highlighted account' : delayed ? 'Existing access remains available' : 'No issues detected';
      setText('tile-accounts', String(data.connections.length)); setText('tile-connected', connected + ' connected'); setText('tile-selected', String(selectedCount())); setText('tile-assets', selectedCount() + ' of ' + assetCount() + ' assets enabled'); setText('tile-services', String(serviceCount())); setText('tile-health', health); setText('tile-health-detail', detail);
    }
    function renderStateBanner() {
      var target = el('state-banner'); target.textContent = ''; target.hidden = true;
      var attention = data.connections.find(function (item) { return item.status !== 'active'; });
      var delayed = attention ? null : data.connections.find(function (item) { return (item.discovery.status === 'error' || item.discovery.status === 'unavailable') && (item.resources || []).length; });
      var connection = attention || delayed; if (!connection) return;
      var copy = document.createElement('div'); var heading = document.createElement('strong'); var detail = document.createElement('p');
      if (attention) { heading.textContent = (connection.email || connection.displayName || 'Google account') + ' needs attention'; detail.textContent = 'Google needs you to reconnect before access can be refreshed. Existing selections are preserved.'; }
      else { heading.textContent = 'Some Google services couldn’t be checked'; detail.textContent = 'Your existing selections are unchanged and remain available.'; }
      copy.appendChild(heading); copy.appendChild(detail);
      var action = document.createElement('button'); action.type = 'button'; action.className = 'button primary'; action.textContent = attention ? 'Reconnect Google' : 'Try again'; action.addEventListener('click', function () { if (attention) reconnect(connection); else discover(connection); });
      target.appendChild(copy); target.appendChild(action); target.hidden = false;
    }
    function renderServiceOverview() {
      var target = el('service-overview-rows'); if (!target) return; target.textContent = ''; var services = new Map();
      data.connections.forEach(function (connection) {
        (connection.resources || []).forEach(function (resource) { if (!services.has(resource.service)) services.set(resource.service, { service: resource.service, label: resource.serviceLabel || label(resource.service), total: 0, selected: 0, accounts: new Set(), delayed: false, attention: false }); var item = services.get(resource.service); item.total += 1; if (resource.enabled) item.selected += 1; item.accounts.add(connection.id); if (connection.discovery.status === 'error' || connection.discovery.status === 'unavailable') item.delayed = true; if (connection.status !== 'active') item.attention = true; });
        (connection.services || []).forEach(function (name) { var key = serviceKey(name); if (!key || services.has(key)) return; services.set(key, { service: key, label: name, total: 0, selected: 0, accounts: new Set([connection.id]), delayed: connection.discovery.status === 'error' || connection.discovery.status === 'unavailable', attention: connection.status !== 'active' }); });
      });
      if (!services.size) { target.innerHTML = '<div class="empty"><h2>No Google services connected yet</h2><p>Connect an account to see its approved services here.</p></div>'; return; }
      Array.from(services.values()).sort(function (a, b) { return category(a.service).localeCompare(category(b.service)) || a.label.localeCompare(b.label); }).forEach(function (item) {
        var row = document.createElement('div'); row.className = 'service-row'; var statusClass = item.attention ? 'needs_attention' : item.delayed ? 'unavailable' : 'ok'; var statusLabel = item.attention ? 'Needs attention' : item.delayed ? 'Check delayed' : 'Connected';
        row.innerHTML = '<div class="service-identity">' + serviceMark(item.service, item.label) + '<div><strong>' + item.label + '</strong><small>' + category(item.service) + ' access</small></div></div><div class="service-cell"><strong>' + item.accounts.size + '</strong><small>accounts</small></div><div class="service-cell selected-cell"><strong>' + item.selected + ' enabled</strong><small>' + item.total + ' available</small></div><div class="service-cell status-cell"><span class="status ' + statusClass + '">' + statusLabel + '</span></div><span class="row-chevron" aria-hidden="true">›</span>'; target.appendChild(row);
      });
    }
    function renderAccountOverview() {
      var target = el('account-overview'); if (!target) return; target.textContent = ''; data.connections.forEach(function (connection) { var entry = document.createElement('div'); entry.className = 'account-entry'; var copy = document.createElement('div'); copy.className = 'account-copy'; var strong = document.createElement('strong'); strong.textContent = connection.email || connection.displayName || 'Google account'; var small = document.createElement('small'); small.textContent = (connection.services || []).length + ' services · ' + (connection.resources || []).filter(function (resource) { return resource.enabled; }).length + ' assets enabled'; copy.appendChild(strong); copy.appendChild(small); var statusData = safeStatus(connection); var badge = document.createElement('span'); badge.className = 'status ' + statusData[0]; badge.textContent = statusData[1]; entry.appendChild(copy); entry.appendChild(badge); target.appendChild(entry); });
    }
    function renderAccounts() {
      var target = el('google-accounts'); if (!target) return; target.textContent = '';
      data.connections.forEach(function (connection) {
        var card = document.createElement('article'); card.className = 'card account-card'; var avatar = document.createElement('span'); avatar.className = 'avatar'; avatar.textContent = (connection.email || 'G').slice(0, 1).toUpperCase(); var info = document.createElement('div'); var title = document.createElement('h3'); title.textContent = connection.email || connection.displayName || 'Google account'; var subtitle = document.createElement('p'); subtitle.className = 'muted'; subtitle.textContent = (connection.services || []).length + ' services · ' + (connection.resources || []).filter(function (resource) { return resource.enabled; }).length + ' assets enabled'; var status = document.createElement('p'); status.className = 'account-status'; var statusData = safeStatus(connection); var badge = document.createElement('span'); badge.className = 'status ' + statusData[0]; badge.textContent = statusData[1]; status.appendChild(badge); info.appendChild(title); info.appendChild(subtitle); info.appendChild(status);
        var actions = document.createElement('div'); actions.className = 'card-actions'; var manage = document.createElement('button'); manage.type = 'button'; manage.className = 'button' + (connection.status === 'active' ? ' primary' : ''); manage.textContent = connection.status === 'active' ? 'Manage data access' : 'Manage access'; manage.addEventListener('click', function () { selectAccount(connection.id); }); actions.appendChild(manage);
        if (connection.status !== 'active') { var reconnectButton = document.createElement('button'); reconnectButton.type = 'button'; reconnectButton.className = 'button attention'; reconnectButton.textContent = 'Reconnect Google'; reconnectButton.addEventListener('click', function () { reconnect(connection); }); actions.appendChild(reconnectButton); }
        var menu = document.createElement('details'); menu.className = 'context-menu'; var summary = document.createElement('summary'); summary.setAttribute('aria-label', 'More account actions'); summary.textContent = '⋮'; menu.appendChild(summary); var menuBody = document.createElement('div'); menuBody.className = 'context-menu-menu';
        function menuAction(buttonLabel, kind, listener) { var button = document.createElement('button'); button.type = 'button'; button.textContent = buttonLabel; if (kind) button.className = kind; button.addEventListener('click', function () { menu.open = false; listener(); }); menuBody.appendChild(button); }
        menuAction('Refresh connection', '', function () { refresh(connection); });     menuAction('Check assets', '', function () { discover(connection); }); menuAction('Update Google permissions', '', function () { reconnect(connection); }); menuAction('Disconnect account', 'danger', function () { disconnect(connection); }); menu.appendChild(menuBody); actions.appendChild(menu); card.appendChild(avatar); card.appendChild(info); card.appendChild(actions);
        if (connection.discovery && (connection.discovery.status === 'error' || connection.discovery.status === 'unavailable') && (connection.resources || []).length) { var partial = document.createElement('div'); partial.className = 'partial-banner'; partial.innerHTML = '<div><strong>Some Google services couldn’t be checked</strong><p>Your existing selections are unchanged and remain available.</p></div>'; var retry = document.createElement('button'); retry.type = 'button'; retry.textContent = 'Try again'; retry.addEventListener('click', function () { discover(connection); }); partial.appendChild(retry); card.appendChild(partial); }
        target.appendChild(card);
      });
      updateTiles(); renderStateBanner(); renderServiceOverview(); renderAccountOverview();
    }
    function serviceGroups(connection) { var groups = new Map(); (connection.resources || []).forEach(function (resource) { var key = resource.service; if (!groups.has(key)) groups.set(key, { service: key, label: resource.serviceLabel || label(key), resources: [] }); groups.get(key).resources.push(resource); }); return Array.from(groups.values()).sort(function (a, b) { return category(a.service).localeCompare(category(b.service)) || a.label.localeCompare(b.label); }); }
    function renderAssets() {
      var connection = selectedAccount(); var workspace = el('asset-workspace'); if (!connection) { workspace.hidden = true; return; } workspace.hidden = false; var groups = serviceGroups(connection); if (!groups.length) { el('asset-rail').textContent = ''; el('asset-groups').innerHTML = '<div class="empty"><h2>No assets found yet</h2><p>This account is connected. Check again when Google has assets available.</p></div>'; return; }
      if (!activeService || !groups.some(function (group) { return group.service === activeService; })) activeService = groups[0].service; var rail = el('asset-rail'); rail.textContent = ''; groups.forEach(function (group) { var item = document.createElement('button'); item.type = 'button'; item.className = 'rail-item' + (group.service === activeService ? ' is-active' : ''); item.innerHTML = serviceMark(group.service, group.label) + '<span>' + group.label + '</span><span>' + group.resources.length + '</span>'; item.addEventListener('click', function () { activeService = group.service; renderAssets(); }); rail.appendChild(item); });
      var query = (el('asset-search').value || '').trim().toLowerCase(); var activeGroup = groups.find(function (group) { return group.service === activeService; }); var target = el('asset-groups'); target.textContent = ''; var block = document.createElement('section'); block.className = 'service-block'; var head = document.createElement('div'); head.className = 'card-head'; var heading = document.createElement('h3'); heading.textContent = activeGroup.label; var meta = document.createElement('p'); meta.className = 'service-meta'; meta.textContent = activeGroup.resources.length + ' assets available'; head.appendChild(heading); head.appendChild(meta); block.appendChild(head);
      var visible = activeGroup.resources.filter(function (resource) { return !query || (resource.displayName + ' ' + resource.parent + ' ' + resource.resourceId).toLowerCase().includes(query); }); var rows = document.createElement('div'); rows.className = 'rows';
      var draft = draftFor(connection);
      visible.forEach(function (resource) { var row = document.createElement('div'); row.className = 'row'; var input = document.createElement('input'); input.type = 'checkbox'; input.checked = draft.get(resourceKey(resource)) === true; var rowId = 'asset-' + connection.id + '-' + resource.service + '-' + resource.resourceId; input.id = rowId; input.setAttribute('aria-label', (resource.displayName || resource.resourceId) + ' access'); input.addEventListener('change', function () { draft.set(resourceKey(resource), input.checked); updateAssetSummary(); }); var choice = document.createElement('label'); choice.className = 'row-label'; choice.htmlFor = rowId; var strong = document.createElement('strong'); strong.textContent = resource.displayName || resource.resourceId; var small = document.createElement('small'); small.textContent = resource.resourceType + (resource.parent ? ' · ' + resource.parent : ''); choice.appendChild(strong); choice.appendChild(small); var side = document.createElement('div'); side.className = 'row-side'; side.innerHTML = '<span class="status ok">Available</span>'; row.appendChild(input); row.appendChild(choice); row.appendChild(side); rows.appendChild(row); });
      if (!visible.length) rows.innerHTML = '<div class="empty"><h2>No matching assets</h2><p>No assets match this search. Existing selections are unchanged.</p></div>'; block.appendChild(rows); target.appendChild(block); updateAssetSummary();
    }
    function updateAssetSummary() { var connection = selectedAccount(); if (!connection) return; var draft = draftFor(connection); var selected = Array.from(draft.values()).filter(Boolean).length; setText('asset-selected-count', selected + ' selected'); var partial = connection.discovery.status === 'error' || connection.discovery.status === 'unavailable'; setText('asset-health', partial ? '● Check delayed' : '● All services healthy'); }
    function selectAccount(connectionId) {
      if (connectionId && connectionId !== selectedId && toolChoicesDirty && !window.confirm('Discard unsaved tool permission changes for this account?')) return;
      selectedId = connectionId; activeService = ''; renderAssets(); renderToolPermissions(); el('google-data').scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
    var toolChoices = new Map(); var toolChoicesConnectionId = null; var toolChoicesDirty = false;
    async function renderToolPermissions() {
      var connection = selectedAccount(); var workspace = el('tool-workspace'); if (!connection) { workspace.hidden = true; return; }
      workspace.hidden = false; var connectionId = connection.id; var target = el('tool-choices'); target.textContent = 'Loading tool permissions…'; el('save-tools').disabled = true; el('tool-status').textContent = ''; toolChoices.clear(); toolChoicesConnectionId = null; toolChoicesDirty = false;
      try {
        var response = await fetch('/api/google/tools?connectionId=' + encodeURIComponent(connectionId)); if (!response.ok) throw new Error('Could not load tool permissions.'); var permissions = await response.json(); if (!selectedAccount() || selectedAccount().id !== connectionId) return;
        target.textContent = ''; toolChoicesConnectionId = connectionId; el('save-tools').disabled = false;
        permissions.tools.forEach(function (tool) {
          var row = document.createElement('div'); row.className = 'tool-choice'; var labelNode = document.createElement('label'); var input = document.createElement('input'); input.type = 'checkbox'; input.checked = tool.enabled; input.dataset.toolName = tool.name;
          var choice = { toolName: tool.name, enabled: tool.enabled, allowDestructive: tool.allowDestructive }; toolChoices.set(tool.name, choice);
          var destructiveInput = null;
          input.addEventListener('change', function () { if (input.checked && tool.write && !window.confirm('Allow ' + tool.name + ' to change Google data for this account?')) input.checked = false; choice.enabled = input.checked; toolChoicesDirty = true; el('tool-status').textContent = 'Unsaved tool permission changes.'; if (!input.checked) { choice.allowDestructive = false; if (destructiveInput) { destructiveInput.checked = false; destructiveInput.disabled = true; } } else if (destructiveInput) destructiveInput.disabled = false; });
          labelNode.appendChild(input); labelNode.append(document.createTextNode(tool.name + (tool.write ? ' · writes' : ' · reads') + ' — ' + tool.description)); row.appendChild(labelNode);
          if (tool.write) { var destructiveLabel = document.createElement('label'); destructiveInput = document.createElement('input'); destructiveInput.type = 'checkbox'; destructiveInput.checked = tool.allowDestructive; destructiveInput.disabled = !tool.enabled; destructiveInput.addEventListener('change', function () { if (destructiveInput.checked && !window.confirm('Allow destructive replacement/deletion for ' + tool.name + '? This is a separate account-specific permission.')) destructiveInput.checked = false; choice.allowDestructive = destructiveInput.checked; toolChoicesDirty = true; el('tool-status').textContent = 'Unsaved tool permission changes.'; }); destructiveLabel.appendChild(destructiveInput); destructiveLabel.append(document.createTextNode('Allow destructive operations for ' + tool.name)); row.appendChild(destructiveLabel); }
          target.appendChild(row);
        });
      } catch (error) { if (selectedAccount() && selectedAccount().id === connectionId) { target.textContent = ''; el('tool-status').textContent = error.message || 'Could not load tools. Existing grants are unchanged.'; } }
    }
    async function jsonFetch(url, body, signal) { return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body || {}), signal: signal }); }
    async function loadResources() {
      el('load-error').hidden = true; el('loading-state').hidden = false; el('workspace-content').hidden = true; try { var response = await fetch('/api/google/resources'); if (!response.ok) throw new Error(); data = await response.json(); if (!data.connections || !data.connections.length) { data.connections = []; selectedId = null; } else if (!selectedId || !data.connections.some(function (item) { return item.id === selectedId; })) selectedId = data.connections[0].id; data.connections.forEach(draftFor); el('loading-state').hidden = true; el('workspace-content').hidden = false; renderAccounts(); renderAssets(); await renderToolPermissions(); return true; } catch { el('loading-state').hidden = true; el('load-error').hidden = false; return false; }
    }
    async function guardedAction(actionLabel, connection, request) {
      message(''); try { var response = await request(); if (!response.ok) { var failure = await response.json().catch(function () { return {}; }); throw new Error(failure.error || actionLabel + ' failed.'); } var reloaded = await loadResources(); if (!reloaded) return false; message(actionLabel + ' complete.', 'info'); return true; } catch (error) { message(error.message || actionLabel + ' failed.', 'error'); return false; }
    }
    var connectionTarget = null;
    function setConnectTarget(connection) {
      connectionTarget = connection ? connection.id : null;
      var button = el('connect-google'); var note = el('connect-target');
      if (!connection) { button.disabled = false; setText('connect-google-label', 'Connect Google account'); note.hidden = true; note.textContent = ''; return; }
      var who = connection.email || connection.displayName || 'this Google account';
      setText('connect-google-label', 'Update Google permissions');
      note.hidden = false; note.textContent = 'Updating permissions for ' + who + '. Existing approvals are preserved; select additional services, then submit to send this account back through Google.';
    }
    function reconnect(connection) {
      setConnectTarget(connection);
      el('connect-panel').scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
    function refresh(connection) { guardedAction('Refresh', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/refresh'); }); }
    function discover(connection) { guardedAction('Asset check', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/discover'); }); }
    function disconnect(connection) { var confirmed = window.confirm('Disconnect ' + (connection.email || connection.displayName || 'this Google account') + '? Agents will lose access.'); if (!confirmed) return; guardedAction('Disconnect', connection, function () { return jsonFetch('/api/google/connections/' + encodeURIComponent(connection.id) + '/disconnect'); }); }
    async function save() {
      var connection = selectedAccount(); if (!connection) return; var draft = draftFor(connection);
      var grants = (connection.resources || []).filter(function (resource) { return draft.get(resourceKey(resource)) !== resource.enabled; }).map(function (resource) { return { service: resource.service, resourceId: resource.resourceId, enabled: draft.get(resourceKey(resource)) === true }; });
      if (!grants.length) { message('No changes to save.', 'info'); return; }
      var connectionId = connection.id; var saved = await guardedAction('Access saved', connection, function () { return jsonFetch('/api/google/grants', { connectionId: connectionId, grants: grants }); });
      if (saved) { assetDrafts.delete(connectionId); if (selectedAccount() && selectedAccount().id === connectionId) renderAssets(); }
    }
    async function saveTools() {
      var connection = selectedAccount(); if (!connection || toolChoicesConnectionId !== connection.id) return; var connectionId = connection.id;
      var grants = Array.from(toolChoices.values()).map(function (choice) { return { toolName: choice.toolName, enabled: choice.enabled, allowDestructive: choice.allowDestructive }; }); el('tool-status').textContent = 'Saving…';
      try { var response = await jsonFetch('/api/google/tools', { connectionId: connectionId, grants: grants }); if (!response.ok) { var result = await response.json(); throw new Error(result.error || 'Could not save tool permissions.'); } if (selectedAccount() && selectedAccount().id === connectionId) { await renderToolPermissions(); el('tool-status').textContent = 'Tool permissions saved for this account.'; } } catch (error) { if (selectedAccount() && selectedAccount().id === connectionId) el('tool-status').textContent = error.message || 'Could not save tools.'; }
    }
    var pickerLibraryPromise = null;
    function loadPickerLibrary() {
      if (!pickerLibraryPromise) pickerLibraryPromise = new Promise(function (resolve, reject) { var script = document.createElement('script'); script.src = 'https://apis.google.com/js/api.js'; script.onload = function () { gapi.load('picker', { callback: resolve, onerror: function () { pickerLibraryPromise = null; reject(new Error('Google Picker could not load.')); } }); }; script.onerror = function () { pickerLibraryPromise = null; reject(new Error('Google Picker could not load.')); }; document.head.appendChild(script); }); return pickerLibraryPromise;
    }
    async function pickFiles() {
      var connection = selectedAccount(); if (!connection) return; var connectionId = connection.id; var status = el('picker-status'); status.textContent = 'Preparing Google Picker for ' + connection.email + '…';
      try {
        await loadPickerLibrary(); var response = await jsonFetch('/api/google/picker/start', { connectionId: connectionId }); var config = await response.json(); if (!response.ok) throw new Error(config.error || 'Picker is not configured. The operator must configure its restricted API key/project number; reconnect with selected-file consent if needed.');
        var picker = new google.picker.PickerBuilder().setDeveloperKey(config.apiKey).setAppId(config.appId).setOAuthToken(config.accessToken).setOrigin(window.location.origin).addView(new google.picker.DocsView(google.picker.ViewId.DOCS)).enableFeature(google.picker.Feature.MULTISELECT_ENABLED).setCallback(async function (result) {
          if (result.action === google.picker.Action.CANCEL) { config.accessToken = ''; status.textContent = 'File selection cancelled. Existing access is unchanged.'; return; } if (result.action !== google.picker.Action.PICKED) return; config.accessToken = '';
          var ids = result[google.picker.Response.DOCUMENTS].map(function (item) { return item[google.picker.Document.ID]; });
          try { var verified = await jsonFetch('/api/google/picker/complete', { connectionId: connectionId, pickerSession: config.pickerSession, fileIds: ids }); var saved = await verified.json(); if (!verified.ok) throw new Error(saved.error || 'Selected files could not be verified. Open Picker again to retry.'); await loadResources(); status.textContent = saved.selectedCount + ' files verified for ' + connection.email + '; ' + saved.rejectedCount + ' unavailable. New files remain disabled.'; } catch (error) { status.textContent = error.message || 'File verification failed.'; }
        }).build(); picker.setVisible(true); status.textContent = 'Select files for ' + config.googleEmail + '. No other account will be changed.';
      } catch (error) { status.textContent = error.message || 'Could not open Google Picker.'; }
    }
    function wireConsentActions() {
      var target = el('connect-services'); function consentInputs(permission) { return Array.prototype.slice.call(target.querySelectorAll('input[data-permission="' + permission + '"]')); }
      function consentWriteFor(name) { return target.querySelector('input[data-permission="write"][value="' + name + '"]'); }
      consentInputs('read').forEach(function (input) { input.addEventListener('change', function () { var writeInput = consentWriteFor(input.value); if (writeInput) { writeInput.disabled = !input.checked; if (!input.checked) writeInput.checked = false; } }); });
      var selectAll = el('connect-select-all'); if (selectAll) selectAll.addEventListener('click', function () { consentInputs('read').forEach(function (input) { input.checked = true; input.dispatchEvent(new Event('change')); }); });
      var clearAll = el('connect-clear-services'); if (clearAll) clearAll.addEventListener('click', function () { consentInputs('read').forEach(function (input) { input.checked = false; input.dispatchEvent(new Event('change')); }); consentInputs('write').forEach(function (writeInput) { writeInput.checked = false; writeInput.disabled = true; }); });
    }


    function selectedWriteServices() { return Array.from(el('connect-services').querySelectorAll('input[data-permission="write"]:checked')).map(function (input) { return input.value; }); }
    function selectedServices() { return Array.from(el('connect-services').querySelectorAll('input[data-permission="read"]:checked')).map(function (input) { return input.value; }); }
    var connectPending = false;
    function connectFailure(message, kind) { var status = el('connect-status'); status.textContent = message; if (kind) status.className = 'muted ' + kind; }
    async function startConnection(body) {
      var status = el('connect-status'); var button = el('connect-google');
      if (connectPending) { status.textContent = 'A Google connection request is already running. Wait for it to finish or reload the page.'; return; }
      connectPending = true; button.disabled = true; status.textContent = 'Starting Google sign-in…';
      var controller = typeof AbortController === 'function' ? new AbortController() : null;
      var timer = controller ? setTimeout(function () { controller.abort(); }, 20000) : null;
      var timedOut = false;
      try {
        var response = await jsonFetch('/api/google/connect', body, controller ? controller.signal : undefined);
        var result;
        try { result = await response.json(); } catch (parseError) {
          if (controller && controller.signal.aborted) { timedOut = true; throw new Error('The Google sign-in request timed out.'); }
          result = {};
        }
        if (!response.ok || !result.authorizationUrl) throw new Error(result.error || 'Could not start Google connection.');
        clearTimeout(timer); button.disabled = false; window.location.assign(result.authorizationUrl); return;
      } catch (error) {
        var guidance = ' Check your network, then select the button again.';
        var aborted = timedOut || Boolean(controller && controller.signal.aborted);
        var reason = aborted ? 'The Google sign-in request timed out.' : (error && error.message) || 'Could not start Google connection.';
        if (reason.charAt(reason.length - 1) !== '.') reason += '.';
        connectFailure(reason + guidance, 'error');
      } finally { connectPending = false; button.disabled = false; if (timer) clearTimeout(timer); }
    }
    function showOutcome(outcome, connectionId) {
      if (!outcome) return; var messages = { connected: ['Google account connected.', 'info'], consent_denied: ['Google consent was denied. Your previous access was not changed.', ''], operator_config_missing: ['Google connection is not configured yet.', ''], google_unavailable: ['Google is temporarily unavailable. Please try again.', ''], google_api_error: ['Google could not complete the connection. Please try again.', ''], identity_conflict: ['Already connected — this Google account is already available.', 'info'], reconnect_identity_mismatch: ['Choose the original Google account for this connection, or add this account separately.', 'error'], state_expired: ['The connection attempt expired. Please start again.', ''], needs_reconnect: ['This Google account needs to be reconnected.', 'error'], state_replayed: ['This connection attempt was already completed.', 'info'] }; var entry = messages[outcome] || ['The Google connection attempt could not be completed.', '']; if (outcome === 'connected') {
        var connection = data.connections.find(function (item) { return item.id === connectionId; }) || data.connections[data.connections.length - 1]; var ready = connection && selectedCountFor(connection) > 0; var completion = el('completion-state'); completion.hidden = false; completion.dataset.ready = String(Boolean(ready)); completion.dataset.connectionId = connection ? connection.id : '';
        setText('completion-accounts', String(data.connections.length)); setText('completion-services', String(connection ? (connection.services || []).length : 0)); setText('completion-assets', String(connection ? selectedCountFor(connection) : 0));
        setText('completion-heading', ready ? 'Your Google data is ready' : 'Google connected — choose your data');
        setText('completion-description', ready ? 'Only the assets you selected are available to connected AI tools.' : 'No assets are available to connected AI tools until you make a selection.');
        setText('completion-continue', ready ? 'Go to workspace →' : 'Choose data →');
        el('completion-step-data').className = 'progress-step' + (ready ? ' done' : ''); el('completion-step-data-dot').textContent = ready ? '✓' : '2'; el('completion-step-ready').className = 'progress-step' + (ready ? ' done' : '');
        completion.scrollIntoView({ behavior: 'smooth', block: 'start' });
      } message(entry[0], entry[1]); window.history.replaceState(null, '', '/');
    }
    window.addEventListener('load', async function () {
      await Clerk.load({ ui: { ClerkUI: window.__internal_ClerkUICtor } }); var userButton = el('user-button'); if (userButton) Clerk.mountUserButton(userButton); el('sign-out').addEventListener('click', function () { return Clerk.signOut({ redirectUrl: '/' }); });
      var sidebar = el('app-sidebar'); var mobileNav = window.matchMedia('(max-width: 900px)'); function updateNavMode() { sidebar.inert = mobileNav.matches && !el('app-shell').classList.contains('nav-open'); sidebar.setAttribute('aria-hidden', String(sidebar.inert)); } updateNavMode(); mobileNav.addEventListener('change', updateNavMode);
      el('nav-toggle').addEventListener('click', function () { var open = el('app-shell').classList.toggle('nav-open'); this.setAttribute('aria-expanded', String(open)); updateNavMode(); }); document.querySelectorAll('.side-nav a, .bottom-nav a').forEach(function (link) { link.addEventListener('click', function () { el('app-shell').classList.remove('nav-open'); el('nav-toggle').setAttribute('aria-expanded', 'false'); updateNavMode(); }); }); el('connect-google').addEventListener('click', function () { var body = { services: selectedServices(), writeServices: selectedWriteServices() }; if (connectionTarget) body.connectionId = connectionTarget; return startConnection(body); }); el('add-connection').addEventListener('click', function () { setConnectTarget(null); el('connect-status').textContent = ''; el('connect-panel').scrollIntoView({ behavior: 'smooth', block: 'start' }); }); el('view-assets').addEventListener('click', function () { selectAccount(selectedId || (data.connections[0] && data.connections[0].id)); }); el('manage-services').addEventListener('click', function () { el('asset-workspace').scrollIntoView({ behavior: 'smooth', block: 'start' }); }); el('manage-accounts').addEventListener('click', function () { el('google-data').scrollIntoView({ behavior: 'smooth', block: 'start' }); }); el('retry-load').addEventListener('click', loadResources); el('save-grants').addEventListener('click', save); el('save-tools').addEventListener('click', saveTools); el('pick-files').addEventListener('click', pickFiles); el('asset-search').addEventListener('input', renderAssets); el('clear-search').addEventListener('click', function () { el('asset-search').value = ''; renderAssets(); }); el('completion-continue').addEventListener('click', function () { var ready = el('completion-state').dataset.ready === 'true'; var connectionId = el('completion-state').dataset.connectionId; el('completion-state').hidden = true; if (!ready && connectionId) { selectedId = connectionId; activeService = ''; renderAssets(); renderToolPermissions(); } (ready ? el('home') : el('asset-workspace')).scrollIntoView({ behavior: 'smooth', block: 'start' }); }); document.querySelector('.toast-close').addEventListener('click', function () { message(''); }); var copyButton = el('copy-mcp-url'); if (copyButton) copyButton.addEventListener('click', function () { navigator.clipboard.writeText(el('mcp-url').textContent); copyButton.textContent = 'Copied'; setTimeout(function () { copyButton.textContent = 'Copy URL'; }, 1800); }); wireConsentActions(); await loadResources(); var location = new URL(window.location.href); showOutcome(location.searchParams.get('google'), location.searchParams.get('connection'));
    });
  </script>`;
  return appShell(content, htmlConfig);
}
