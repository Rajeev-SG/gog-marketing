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

/** Calls the private #63 Go execution service's typed discovery endpoint. */
export class RemoteDiscoveryRunner implements DiscoveryRunner {
  private readonly url: string;
  private readonly token: string;
  private readonly fetchImpl: typeof fetch;

  constructor(options: { url: string; token: string; fetchImpl?: typeof fetch }) {
    this.url = options.url;
    this.token = options.token;
    this.fetchImpl = options.fetchImpl ?? fetch.bind(globalThis);
  }

  async discover(request: DiscoveryRequest): Promise<DiscoveryOutcome> {
    let response: Response;
    try {
      response = await this.fetchImpl(this.url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${this.token}`,
        },
        body: JSON.stringify({
          tenant_id: request.tenantId,
          connection_id: request.connectionId,
          services: request.services,
        }),
        signal: AbortSignal.timeout(20_000),
      });
    } catch {
      return { status: "error", detail: "runner_unreachable", resources: [], statuses: {} };
    }
    if (response.status < 200 || response.status >= 300) {
      return { status: "error", detail: "runner_unreachable", resources: [], statuses: {} };
    }
    let wire: WireReport;
    try {
      wire = (await response.json()) as WireReport;
    } catch {
      return { status: "error", detail: "runner_invalid_response", resources: [], statuses: {} };
    }
    return mapWireReport(wire);
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
