/**
 * Hosted v1 Google scope catalog.
 *
 * Mirrors the Go control-plane's read-only selection
 * (`googleauth.ScopesForManageWithOptions{Readonly: true}`) for the hosted
 * product surface: the resource-model services (analytics, googleads,
 * tagmanager, searchconsole, bigquery) plus the workspace tool services
 * (gmail, calendar, drive). Identity scopes follow the Go provider
 * (`openid email userinfo.email`) and are always requested so the returned
 * Google identity can be verified before any connection is created or
 * replaced.
 *
 * Scopes are requested incrementally per enabled service — the hosted connect
 * flow never asks for the whole gog scope universe, and no write
 * scope (gmail.modify, drive, calendar, analytics.edit, ...) is offered.
 * Read-only Gmail/Drive scopes remain RESTRICTED under Google policy: this
 * catalog is for the deliberate two-user Testing owner cohort, not public
 * launch approval. Public-beta verification/scope gating remains #69.
 */

export const IDENTITY_SCOPES: readonly string[] = [
  "openid",
  "email",
  "https://www.googleapis.com/auth/userinfo.email",
] as const;

export interface HostedServiceInfo {
  label: string;
  scopes: string[];
  /** Resource-backed services discover concrete resources; tool services do not. */
  resourceModel: boolean;
}

export const HOSTED_SERVICES: Readonly<Record<string, HostedServiceInfo>> = {
  analytics: {
    label: "Google Analytics 4",
    scopes: ["https://www.googleapis.com/auth/analytics.readonly"],
    resourceModel: true,
  },
  googleads: {
    label: "Google Ads",
    scopes: ["https://www.googleapis.com/auth/adwords"],
    resourceModel: true,
  },
  tagmanager: {
    label: "Google Tag Manager",
    scopes: ["https://www.googleapis.com/auth/tagmanager.readonly"],
    resourceModel: true,
  },
  searchconsole: {
    label: "Search Console",
    scopes: ["https://www.googleapis.com/auth/webmasters.readonly"],
    resourceModel: true,
  },
  bigquery: {
    label: "BigQuery",
    scopes: ["https://www.googleapis.com/auth/bigquery.readonly"],
    resourceModel: true,
  },
  gmail: {
    label: "Gmail",
    scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
    resourceModel: false,
  },
  calendar: {
    label: "Calendar",
    scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
    resourceModel: false,
  },
  drive: {
    label: "Drive",
    scopes: ["https://www.googleapis.com/auth/drive.readonly"],
    resourceModel: false,
  },
} as const;

export class UnknownServiceError extends Error {}

export function isHostedService(service: string): boolean {
  return Object.hasOwn(HOSTED_SERVICES, service);
}

/** Normalize an optional service list; rejects unknown services. */
export function normalizeServices(raw: unknown): string[] {
  if (raw === undefined || raw === null) return [];
  if (!Array.isArray(raw)) {
    throw new UnknownServiceError("services must be a list of service names");
  }
  const out: string[] = [];
  for (const value of raw) {
    const service = String(value ?? "")
      .trim()
      .toLowerCase();
    if (!service) continue;
    if (!isHostedService(service)) {
      throw new UnknownServiceError("unsupported service");
    }
    if (!out.includes(service)) out.push(service);
  }
  return out;
}

/**
 * Sorted, de-duplicated scope set for identity + selected services.
 * Mirrors Go's mergeScopes ordering so the URL is stable.
 */
export function scopesForServices(services: string[]): string[] {
  const set = new Set<string>(IDENTITY_SCOPES);
  for (const service of services) {
    const info = HOSTED_SERVICES[service];
    if (!info) throw new UnknownServiceError("unsupported service");
    for (const scope of info.scopes) set.add(scope);
  }
  return [...set].sort();
}

/** Merge granted scopes into an existing sorted grant list. */
export function mergeScopes(existing: string[], granted: string[]): string[] {
  const set = new Set<string>();
  for (const scope of [...existing, ...granted]) {
    const trimmed = String(scope ?? "").trim();
    if (trimmed) set.add(trimmed);
  }
  return [...set].sort();
}
