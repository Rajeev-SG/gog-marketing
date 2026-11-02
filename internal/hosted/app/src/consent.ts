import { HOSTED_SERVICES } from "./scopes.js";

/**
 * Consent-form slice: the custodian-priority services only, grouped for the
 * connect form. Deferred services (contacts, YouTube, …) stay in
 * HOSTED_SERVICES for the allowlist and existing encrypted grants, but are not
 * offered in the connect-service consent UI.
 */
export const CONSENT_GROUPS: ReadonlyArray<{ legend: string; services: readonly string[] }> = [
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
  return CONSENT_GROUPS.flatMap((group) => [...group.services]);
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
