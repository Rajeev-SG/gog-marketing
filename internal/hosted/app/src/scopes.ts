import serviceCatalog from "../../policy/services.json";

/** Hosted-only audit-free consent policy (#79). Eligibility is not executable
 * tool coverage or Google production approval. Never inherit CLI OAuth defaults. */
export const IDENTITY_SCOPES: readonly string[] = [
  "openid",
  "email",
  "https://www.googleapis.com/auth/userinfo.email",
] as const;

export interface HostedServiceInfo {
  label: string;
  scopes: string[];
  /** Additional consent only for explicitly selected write services. */
  writeScopes: string[];
  resourceModel: boolean;
  documentation: string;
}
export const HOSTED_SERVICES: Readonly<Record<string, HostedServiceInfo>> = serviceCatalog;

export class UnknownServiceError extends Error {}
export class UnsafeHostedScopeError extends Error {}
const ALLOWED_SCOPES = new Set([
  ...IDENTITY_SCOPES,
  "https://www.googleapis.com/auth/userinfo.profile", // Google's canonical alias for profile.
  ...Object.values(HOSTED_SERVICES).flatMap((info) => [...info.scopes, ...info.writeScopes]),
]);
/** Exact allowlist rejects unknown, restricted and transitive additions. */
export function assertHostedScopes(values: readonly string[]): void {
  if (!values.length || !values.every((scope) => ALLOWED_SCOPES.has(scope))) {
    throw new UnsafeHostedScopeError("Google grant contains scopes outside the hosted allowlist");
  }
}

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
    if (typeof value !== "string") throw new UnknownServiceError("invalid service name");
    const service = value.trim().toLowerCase();
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
export function scopesForServices(services: string[], writeServices: string[] = []): string[] {
  const set = new Set<string>(IDENTITY_SCOPES);
  for (const service of services) {
    const info = HOSTED_SERVICES[service];
    if (!isHostedService(service)) throw new UnknownServiceError("unsupported service");
    for (const scope of info.scopes) set.add(scope);
  }
  for (const service of writeServices) {
    if (
      !isHostedService(service) ||
      !services.includes(service) ||
      !HOSTED_SERVICES[service]!.writeScopes.length
    ) {
      throw new UnknownServiceError("write service must be selected and support writes");
    }
    for (const scope of HOSTED_SERVICES[service]!.writeScopes) set.add(scope);
  }
  const result = [...set].sort();
  assertHostedScopes(result);
  return result;
}

/** Merge granted scopes into an existing sorted grant list. */
export function mergeScopes(existing: string[], granted: string[]): string[] {
  const set = new Set<string>();
  for (const scope of [...existing, ...granted]) {
    const trimmed = String(scope ?? "").trim();
    if (trimmed) set.add(trimmed);
  }
  const result = [...set].sort();
  assertHostedScopes(result);
  return result;
}
