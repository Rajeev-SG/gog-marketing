import vm from "node:vm";

import { describe, expect, it } from "vitest";

import { consentServiceIds, deferredConsentServiceIds, renderHome } from "./html.js";
import { HOSTED_SERVICES } from "./scopes.js";
import { PRODUCT_CSS } from "./product-css.generated.js";

const config = {
  publishableKey: "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA",
  authorizedParties: ["https://gog-marketing.rajeev-sgill.workers.dev"],
  clerkDomain: "oriented-wombat-2813.clerk.accounts.dev",
  mcpOrigin: "https://gog-marketing.rajeev-sgill.workers.dev",
};

describe("hosted marketer UI reference states (#50)", () => {
  const html = renderHome(config);

  it("uses the shared product design layer and persistent application shell", () => {
    expect(html).toContain(PRODUCT_CSS);
    expect(html).toContain('class="app-shell"');
    expect(html).toContain('class="sidebar"');
    expect(html).toContain('class="bottom-nav"');
    expect(html).toContain("Your Google marketing data");
  });

  it("offers Chat in both desktop and mobile navigation without changing Google settings", () => {
    const desktop = html.match(/<nav class="side-nav">([\s\S]*?)<\/nav>/)?.[1];
    const mobile = html.match(/<nav class="bottom-nav"[^>]*>([\s\S]*?)<\/nav>/)?.[1];
    for (const navigation of [desktop, mobile]) {
      expect(navigation).toContain('href="/chat"');
      expect(navigation).toContain('href="#google-data"');
    }
    expect(PRODUCT_CSS).toContain("grid-auto-flow: column; grid-auto-columns: 1fr");
  });

  it("represents loading, partial failure, completion and duplicate feedback", () => {
    expect(html).toContain('id="loading-state"');
    expect(html).toContain("Finding your Google marketing assets…");
    expect(html).toContain("Some Google services couldn’t be checked");
    expect(html).toContain("Your existing selections are unchanged and remain available.");
    expect(html).toContain('id="state-banner"');
    expect(html).toContain("function selectedCountFor(connection)");
    expect(html).toContain("completion.dataset.connectionId");
    expect(html).toContain('id="completion-state"');
    expect(html).toContain("Your Google data is ready");
    expect(html).toContain("Already connected — this Google account is already available.");
    expect(html).toContain("Choose the original Google account for this connection");
  });

  it("keeps healthy operations contextual and uses marketer-facing status copy", () => {
    expect(html).toContain("menu.className = 'context-menu'");
    expect(html).not.toContain("Discovery ready");
    expect(html).not.toContain("Discovery unavailable");
    expect(html).not.toContain("action('Reconnect'");
    expect(html).not.toContain("action('Refresh'");
    expect(html).not.toContain("action('Discover'");
  });

  it("includes responsive asset and mobile navigation states", () => {
    expect(html).toContain('id="asset-workspace"');
    expect(html).toContain('id="asset-search"');
    expect(html).toContain("@media (max-width: 680px)");
    expect(html).toContain("@media (max-width: 900px)");
  });
});

describe("stalled-account reconnect targeting", () => {
  const page = renderHome(config);

  it("offers Update Google permissions in every account menu", () => {
    expect(page).toContain(
      "menuAction('Update Google permissions', '', function () { reconnect(connection); });",
    );
  });

  it("opens the submit form for the targeted connection instead of issuing OAuth immediately", () => {
    expect(page).toMatch(/function reconnect\(connection\)\s*\{\s*setConnectTarget\(connection\);/);
    expect(page).toMatch(
      /function reconnect\(connection\)\s*\{\s*setConnectTarget\(connection\);[\s\S]*?el\('connect-panel'\)\.scrollIntoView\(\{ behavior: 'smooth', block: 'start' \}\);\s*\}/,
    );
    expect(page).not.toContain("startConnection({ connectionId: connection.id");
  });

  it("shows the targeted account context and swaps the submit button label", () => {
    expect(page).toContain("function setConnectTarget(connection)");
    expect(page).toContain("connectionTarget = connection ? connection.id : null;");
    expect(page).toContain(
      "'Updating permissions for ' + who + '. Existing approvals are preserved;",
    );
    expect(page).toContain("setText('connect-google-label', 'Update Google permissions');");
    expect(page).toContain('id="connect-target"');
    expect(page).toContain('id="connect-google-label"');
  });

  it("sends the target connectionId only on explicit form submit", () => {
    expect(page).toContain(
      "var body = { services: selectedServices(), writeServices: selectedWriteServices() }; if (connectionTarget) body.connectionId = connectionTarget;",
    );
    expect(page).toContain("await jsonFetch('/api/google/connect', body,");
  });

  it("resets to new-account context when Add Google account is used", () => {
    expect(page).toContain(
      "el('add-connection').addEventListener('click', function () { setConnectTarget(null);",
    );
    expect(page).toContain(
      "if (!connection) { button.disabled = false; setText('connect-google-label', 'Connect Google account'); note.hidden = true; note.textContent = ''; return; }",
    );
  });

  it("blocks double submit and always restores the button with an actionable failure", () => {
    expect(page).toContain("var connectPending = false;");
    expect(page).toContain(
      "if (connectPending) { status.textContent = 'A Google connection request is already running. Wait for it to finish or reload the page.'; return; }",
    );
    expect(page).toContain("new AbortController()");
    expect(page).toContain("}, 20000)");
    expect(page).toContain("The Google sign-in request timed out.");
    expect(page).toContain("' Check your network, then select the button again.'");
    expect(page).toContain(
      "finally { connectPending = false; button.disabled = false; if (timer) clearTimeout(timer); }",
    );
  });
});

describe("connect-service consent form", () => {
  const page = renderHome(config);

  it("shows exactly the 13 priority services grouped as Workspace and Marketing", () => {
    expect(page.match(/<fieldset class="service-consent-group"/g)).toHaveLength(2);
    expect(page).toContain("<legend>Workspace</legend>");
    expect(page).toContain("<legend>Marketing</legend>");
    expect(page.match(/class="service-choice"/g)).toHaveLength(13);
    expect(page.match(/id="consent-read-/g)).toHaveLength(13);
    const listed = [...page.matchAll(/data-service="([a-z]+)"/g)].map((match) => match[1]);
    expect(listed).toEqual(consentServiceIds());
    expect(consentServiceIds()).toEqual([
      "calendar",
      "drive",
      "drivelabels",
      "docs",
      "slides",
      "tasks",
      "sheets",
      "forms",
      "appscript",
      "analytics",
      "searchconsole",
      "bigquery",
      "tagmanager",
    ]);
  });

  it("pairs a labelled write control with each writable service and notes read-only ones", () => {
    const writable = consentServiceIds().filter(
      (service) => HOSTED_SERVICES[service]!.writeScopes.length > 0,
    );
    expect(writable).toHaveLength(12);
    expect(page.match(/data-permission="write" disabled/g)).toHaveLength(12);
    for (const service of consentServiceIds()) {
      expect(page).toContain('for="consent-read-' + service + '"');
    }
    for (const service of writable) {
      expect(page).toContain('id="consent-write-' + service + '"');
      expect(page).toContain('for="consent-write-' + service + '"');
    }
    for (const service of consentServiceIds()) {
      expect(page).toContain('id="consent-read-' + service + '"');
    }
    expect(page.match(/class="service-choice-note"/g)).toHaveLength(1);
    expect(page).toContain(">Read only</span>");
  });

  it("keeps deferred services out of this UI but inside the hosted catalog", () => {
    for (const service of deferredConsentServiceIds()) {
      expect(Object.keys(HOSTED_SERVICES)).toContain(service);
      expect(page).not.toContain('data-service="' + service + '"');
      expect(page).not.toContain('id="consent-read-' + service + '"');
    }
    expect(Object.keys(HOSTED_SERVICES)).toHaveLength(23);
  });

  it("starts neutral with select-all and clear wired to read-only semantics", () => {
    expect(page).toContain('id="connect-select-all" type="button">Select all services</button>');
    expect(page).toContain('id="connect-clear-services" type="button">Clear selection</button>');
    expect(page).toContain(
      "13 service choices are shown. Read and write consent controls Google data access only",
    );
    expect(page).not.toMatch(/consent-(read|write)-[a-z]+"[^>]*checked/);
    expect(page).not.toContain(
      'type="checkbox" value="tagmanager" data-permission="write" checked',
    );
    expect(page).toContain("function wireConsentActions()");
    expect(page).not.toContain("renderConsentChoices");
    expect(page).toContain(
      "consentInputs('read').forEach(function (input) { input.checked = true; input.dispatchEvent(new Event('change')); });",
    );
    expect(page).toContain(
      "consentInputs('write').forEach(function (writeInput) { writeInput.checked = false; writeInput.disabled = true; });",
    );
  });

  it("keeps the grouped consent layout responsive on narrow screens", () => {
    expect(page).toContain(".service-consent { grid-template-columns: 1fr; }");
    expect(page).toContain(".service-consent-group { padding: 9px 10px 11px; }");
    expect(page).toContain(".connect-consent-toolbar button { flex: 1 1 auto; }");
  });
});

/** Execute the actual rendered startConnection script fragment in a fresh vm context. */
function executeConnectScript() {
  const page = renderHome(config);
  const start = page.indexOf("var connectPending = false;");
  const end = page.indexOf("function showOutcome");
  expect(start).toBeGreaterThan(-1);
  expect(end).toBeGreaterThan(start);
  const fragment = page.slice(start, end);

  const button = { disabled: false };
  const statusEl = { textContent: "", className: "" };
  const calls: Array<{ url: string; body: unknown; signal?: AbortSignal }> = [];
  const assigned: string[] = [];
  let timerCallback: (() => void) | undefined;
  let respond: ((signal?: AbortSignal) => unknown) | undefined;
  let rejectFetch: ((error: unknown) => void) | undefined;

  const sandbox = {
    el: (id: string) =>
      id === "connect-status" ? statusEl : id === "connect-google" ? button : undefined,
    jsonFetch: async (url: string, body: unknown, signal?: AbortSignal) => {
      calls.push({ url, body, signal });
      if (rejectFetch) throw rejectFetch(undefined);
      if (respond) return respond(signal);
      return new Promise((_, reject) => {
        signal?.addEventListener("abort", () => reject(signal.reason));
      });
    },
    setTimeout: (fn: () => void) => {
      timerCallback = fn;
      return 1;
    },
    clearTimeout: () => {
      timerCallback = undefined;
    },
    AbortController,
    window: { location: { assign: (url: string) => assigned.push(url) } },
  };
  const context = vm.createContext(sandbox);
  vm.runInNewContext(fragment, context, { filename: "html.ts renderHome fragment" });
  const evaluate = (expression: string) => vm.runInContext(expression, context, { timeout: 1000 });
  return {
    button,
    statusEl,
    calls,
    assigned,
    fireTimer: () => {
      expect(timerCallback, "submit timeout timer should be scheduled").toBeTypeOf("function");
      timerCallback?.();
    },
    resolveWith: (factory: (signal?: AbortSignal) => unknown) => {
      respond = factory;
    },
    rejectFetchWith: (factory: () => unknown) => {
      rejectFetch = factory;
    },
    evaluate,
  };
}

describe("startConnection error handling (executed script)", () => {
  it("classifies an abort before headers as a timeout with retry guidance and restores controls", async () => {
    const run = executeConnectScript();
    const pending = run.evaluate(
      "startConnection({ services: ['drive'], writeServices: [] })",
    ) as Promise<unknown>;
    run.fireTimer();
    await pending;
    expect(run.statusEl.textContent).toBe(
      "The Google sign-in request timed out. Check your network, then select the button again.",
    );
    expect(run.statusEl.className).toBe("muted error");
    expect(run.button.disabled).toBe(false);
    expect(run.assigned).toHaveLength(0);
    expect(run.evaluate("connectPending")).toBe(false);
  });

  it("propagates an abort during response body parsing instead of swallowing it", async () => {
    const run = executeConnectScript();
    run.resolveWith((signal) => ({
      ok: true,
      json: () =>
        new Promise((_, reject) => {
          if (signal?.aborted) {
            reject(signal.reason);
            return;
          }
          signal?.addEventListener("abort", () => reject(signal.reason));
        }),
    }));
    const pending = run.evaluate(
      "startConnection({ services: [], writeServices: [] })",
    ) as Promise<unknown>;
    run.fireTimer();
    await pending;
    expect(run.statusEl.textContent).toBe(
      "The Google sign-in request timed out. Check your network, then select the button again.",
    );
    expect(run.assigned).toHaveLength(0);
    expect(run.button.disabled).toBe(false);
  });

  it("appends actionable retry guidance to a network TypeError instead of trusting its message", async () => {
    const run = executeConnectScript();
    run.rejectFetchWith(() => new TypeError("Failed to fetch"));
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.statusEl.textContent).toBe(
      "Failed to fetch. Check your network, then select the button again.",
    );
    expect(run.statusEl.className).toBe("muted error");
    expect(run.button.disabled).toBe(false);
    expect(run.evaluate("connectPending")).toBe(false);
  });

  it("reports an HTTP failure whose error body is malformed JSON, with retry guidance", async () => {
    const run = executeConnectScript();
    run.resolveWith(() => ({
      ok: false,
      json: () => Promise.reject(new SyntaxError("Unexpected token < in JSON")),
    }));
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.statusEl.textContent).toBe(
      "Could not start Google connection. Check your network, then select the button again.",
    );
    expect(run.button.disabled).toBe(false);
    expect(run.evaluate("connectPending")).toBe(false);
  });

  it("appends retry guidance to server error bodies and allows a fresh submission afterwards", async () => {
    const run = executeConnectScript();
    run.resolveWith(() => ({
      ok: false,
      json: () => Promise.resolve({ error: "Invalid service selection." }),
    }));
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.statusEl.textContent).toBe(
      "Invalid service selection. Check your network, then select the button again.",
    );
    run.resolveWith(() => ({
      ok: true,
      json: () =>
        Promise.resolve({ authorizationUrl: "https://accounts.google.com/o/oauth2/auth?x" }),
    }));
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.assigned).toEqual(["https://accounts.google.com/o/oauth2/auth?x"]);
    expect(run.button.disabled).toBe(false);
    expect(run.evaluate("connectPending")).toBe(false);
  });

  it("suppresses a duplicate submission while one is pending, then permits the next", async () => {
    const run = executeConnectScript();
    run.resolveWith((signal) => ({
      ok: true,
      json: () =>
        new Promise((resolve) => {
          signal?.addEventListener("abort", () => resolve({ authorizationUrl: "https://x.test" }));
        }),
    }));
    const pending = run.evaluate(
      "startConnection({ services: ['drive'], writeServices: ['drive'] })",
    ) as Promise<unknown>;
    expect(run.calls).toHaveLength(1);
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.calls).toHaveLength(1);
    expect(run.statusEl.textContent).toBe(
      "A Google connection request is already running. Wait for it to finish or reload the page.",
    );
    run.fireTimer();
    await pending;
    expect(run.assigned).toEqual(["https://x.test"]);
    expect(run.calls).toHaveLength(1);
    run.resolveWith(() => ({
      ok: true,
      json: () => Promise.resolve({ authorizationUrl: "https://again.test" }),
    }));
    await run.evaluate("startConnection({ services: [], writeServices: [] })");
    expect(run.calls).toHaveLength(2);
    expect(run.assigned).toEqual(["https://x.test", "https://again.test"]);
  });
});
