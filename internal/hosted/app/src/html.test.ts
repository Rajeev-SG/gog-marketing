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
