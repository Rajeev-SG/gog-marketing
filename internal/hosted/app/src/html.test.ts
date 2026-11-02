import { describe, expect, it } from "vitest";

import { CONSENT_GROUPS, consentServiceIds, deferredConsentServiceIds } from "./consent.js";
import { renderPublicAbout } from "./html.js";
import { HOSTED_SERVICES } from "./scopes.js";

describe("connect-service consent catalogue", () => {
  it("offers exactly the 13 priority services grouped as Workspace and Marketing", () => {
    expect(CONSENT_GROUPS.map((group) => group.legend)).toEqual(["Workspace", "Marketing"]);
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

  it("has a write choice for every writable service and exactly one read-only service", () => {
    const writable = consentServiceIds().filter(
      (service) => HOSTED_SERVICES[service]!.writeScopes.length > 0,
    );
    expect(writable).toHaveLength(12);
    expect(consentServiceIds().filter((service) => !writable.includes(service))).toHaveLength(1);
  });

  it("keeps deferred services out of the form but inside the hosted catalogue", () => {
    for (const service of deferredConsentServiceIds()) {
      expect(Object.keys(HOSTED_SERVICES)).toContain(service);
      expect(consentServiceIds()).not.toContain(service);
    }
    expect(Object.keys(HOSTED_SERVICES)).toHaveLength(23);
  });
});

it("provides a factual public help page without presenting draft legal policy as approved", () => {
  const page = renderPublicAbout();
  expect(page).toContain("connects the Google accounts you choose to AI chat and tools");
  // Draft legal policy and Google verification must never read as approved.
  expect(page).toContain("still being finalised");
  expect(page).toContain("not a legal policy");
  expect(page).toContain('id="help"');
  expect(page).not.toContain("clerk.browser.js");
  expect(page).toContain('href="/"');
});
