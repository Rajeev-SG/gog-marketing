/**
 * Typed discovery-runner contract for hosted v1.
 *
 * The canonical Google execution/discovery engine stays in Go (#63 Cloud
 * Run). The Worker never re-implements Google discovery: it either calls a
 * configured runner with the Go control-plane wire contract
 * (`internal/controlplane` DiscoveryReport / ResourceGrant JSON shapes) or
 * reports the runner as explicitly unavailable. Failures never fabricate
 * inventory and never erase or expand existing grants.
 */

import { signRunnerCapabilityJwt, type NativeIdentityProvider } from "./runner-auth.js";

export type RunnerServiceState = "ok" | "unavailable" | "unsupported" | "error";

export interface RunnerServiceStatus {
  state: RunnerServiceState;
  detail: string;
  resourceCount: number;
  checkedAt: string;
}

export interface DiscoveredResource {
  service: string;
  resourceType: string;
  resourceId: string;
  displayName: string;
  parent: string;
  metadata: Record<string, string>;
}

export type DiscoveryOutcomeStatus = "ok" | "empty" | "unavailable" | "error";

export interface DiscoveryOutcome {
  status: DiscoveryOutcomeStatus;
  /** Safe detail code; never a raw provider exception. */
  detail: string;
  resources: DiscoveredResource[];
  statuses: Record<string, RunnerServiceStatus>;
}

export interface DiscoveryRequest {
  tenantId: string;
  connectionId: string;
  services: string[];
  accessToken: string;
  /** Trusted verified account email loaded server-side from persisted state. */
  googleEmail: string;
  /** Trusted verified account subject loaded server-side from persisted state. */
  googleSubject: string;
}

export interface DiscoveryRunner {
  discover(request: DiscoveryRequest): Promise<DiscoveryOutcome>;
}

/** Used until the #63 Go discovery runner is deployed/configured. */
export class UnavailableDiscoveryRunner implements DiscoveryRunner {
  async discover(_request: DiscoveryRequest): Promise<DiscoveryOutcome> {
    return {
      status: "unavailable",
      detail: "runner_not_configured",
      resources: [],
      statuses: {},
    };
  }
}

interface WireReport {
  resources?: unknown;
  statuses?: unknown;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isGoDiscoveryResource(value: Record<string, unknown>): boolean {
  return (
    typeof value.service === "string" &&
    value.service !== "" &&
    typeof value.resource_type === "string" &&
    typeof value.resource_id === "string" &&
    value.resource_id !== "" &&
    typeof value.enabled === "boolean"
  );
}

function isGoServiceStatus(value: Record<string, unknown>): boolean {
  return (
    typeof value.state === "string" &&
    ["ok", "unavailable", "unsupported", "error"].includes(value.state) &&
    typeof value.resource_count === "number" &&
    Number.isInteger(value.resource_count) &&
    value.resource_count >= 0 &&
    typeof value.checked_at === "string" &&
    value.checked_at !== ""
  );
}

function isGoDiscoveryEnvelope(
  wire: unknown,
): wire is { operation: string; result: { resources: unknown[]; statuses: unknown } } {
  return (
    isPlainObject(wire) &&
    wire.ok === true &&
    wire.operation === "discover" &&
    isPlainObject(wire.result) &&
    Array.isArray(wire.result.resources) &&
    isPlainObject(wire.result.statuses) &&
    wire.result.resources.every(isGoDiscoveryResource) &&
    Object.values(wire.result.statuses as Record<string, unknown>).every(
      (status) => isPlainObject(status) && isGoServiceStatus(status),
    )
  );
}

/** Calls the private #63 Go execution service's typed discovery endpoint. */
export class RemoteDiscoveryRunner implements DiscoveryRunner {
  private readonly url: URL;
  private readonly invocationToken: string;
  private readonly identity: NativeIdentityProvider;
  private readonly fetchImpl: typeof fetch;

  constructor(options: {
    url: URL;
    invocationToken: string;
    identity: NativeIdentityProvider;
    fetchImpl?: typeof fetch;
  }) {
    this.url = options.url;
    this.invocationToken = options.invocationToken;
    this.identity = options.identity;
    this.fetchImpl = options.fetchImpl ?? fetch.bind(globalThis);
  }

  async discover(request: DiscoveryRequest): Promise<DiscoveryOutcome> {
    // Go explicitly rejects zero-service discovery. A basic identity-only
    // connection legitimately requests none, so keep that as an explicit
    // Worker no-op instead of asking the runner for unsupported work.
    if (request.services.length === 0) {
      return {
        status: "empty",
        detail: "no_discovery_services",
        resources: [],
        statuses: {},
      };
    }

    const deadlineSignal = AbortSignal.timeout(20_000);
    if (deadlineSignal.aborted) throw new Error("discovery deadline exceeded");

    let nativeIdToken: string;
    try {
      nativeIdToken = await this.identity.nativeIdToken(this.url.origin, deadlineSignal);
    } catch {
      return {
        status: "unavailable",
        detail: "operator_identity_unavailable",
        resources: [],
        statuses: {},
      };
    }

    const body: Record<string, unknown> = {
      tenant_id: request.tenantId,
      connection_id: request.connectionId,
      operation: "discover",
      google_email: request.googleEmail,
      google_subject: request.googleSubject,
      access_token: request.accessToken,
      services: request.services,
    };
    const exactBodyBytes = new TextEncoder().encode(JSON.stringify(body));
    const capability = await signRunnerCapabilityJwt(
      this.invocationToken,
      {
        tenantId: request.tenantId,
        connectionId: request.connectionId,
        operation: "discover",
      },
      exactBodyBytes,
    );

    let response: Response;
    try {
      response = await this.fetchImpl(this.url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${capability}`,
          "X-Serverless-Authorization": `Bearer ${nativeIdToken}`,
        },
        body: exactBodyBytes,
        signal: deadlineSignal,
      });
    } catch {
      return { status: "error", detail: "runner_unreachable", resources: [], statuses: {} };
    }
    if (response.status < 200 || response.status >= 300) {
      return { status: "error", detail: "runner_unreachable", resources: [], statuses: {} };
    }
    let wire: unknown;
    try {
      wire = (await response.json()) as WireReport;
    } catch {
      return { status: "error", detail: "runner_invalid_response", resources: [], statuses: {} };
    }
    if (!isGoDiscoveryEnvelope(wire)) {
      return { status: "error", detail: "runner_invalid_response", resources: [], statuses: {} };
    }
    return mapWireReport(wire.result);
  }
}

export function mapWireReport(wire: WireReport): DiscoveryOutcome {
  const resources: DiscoveredResource[] = [];
  const rawResources = Array.isArray(wire.resources) ? wire.resources : [];
  for (const raw of rawResources) {
    if (raw === null || typeof raw !== "object") continue;
    const item = raw as Record<string, unknown>;
    const service = String(item.service ?? "").trim();
    const resourceId = String(item.resource_id ?? "").trim();
    if (!service || !resourceId) continue;
    const metadata: Record<string, string> = {};
    if (item.metadata && typeof item.metadata === "object" && !Array.isArray(item.metadata)) {
      for (const [key, value] of Object.entries(item.metadata as Record<string, unknown>)) {
        metadata[String(key)] = String(value ?? "");
      }
    }
    resources.push({
      service,
      resourceType: String(item.resource_type ?? "").trim(),
      resourceId,
      displayName: String(item.display_name ?? "").trim(),
      parent: String(item.parent ?? "").trim(),
      metadata,
    });
  }

  const statuses: Record<string, RunnerServiceStatus> = {};
  let sawError = false;
  let sawUnavailable = false;
  if (wire.statuses && typeof wire.statuses === "object" && !Array.isArray(wire.statuses)) {
    for (const [service, rawStatus] of Object.entries(wire.statuses as Record<string, unknown>)) {
      if (rawStatus === null || typeof rawStatus !== "object") continue;
      const status = rawStatus as Record<string, unknown>;
      const state = String(status.state ?? "");
      if (state === "error") sawError = true;
      if (state === "unavailable") sawUnavailable = true;
      statuses[service] = {
        state: (["ok", "unavailable", "unsupported", "error"].includes(state)
          ? state
          : "error") as RunnerServiceState,
        detail: String(status.detail ?? "").trim(),
        resourceCount: Number(status.resource_count ?? 0),
        checkedAt: String(status.checked_at ?? "").trim(),
      };
    }
  }

  let outcomeStatus: DiscoveryOutcomeStatus;
  let detail: string;
  if (sawError) {
    outcomeStatus = "error";
    detail = "service_error";
  } else if (resources.length > 0) {
    outcomeStatus = "ok";
    detail = "";
  } else if (sawUnavailable) {
    outcomeStatus = "unavailable";
    detail = "service_unavailable";
  } else {
    outcomeStatus = "empty";
    detail = "no_resources";
  }
  return { status: outcomeStatus, detail, resources, statuses };
}
