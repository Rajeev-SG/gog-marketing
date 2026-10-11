/**
 * Typed MCP tool execution against the private hosted Go runner.
 *
 * Hosted v1 currently exposes exactly one executable MCP mapping:
 * analytics_properties_get -> analytics_property_read. The Worker re-checks
 * tenant/resource policy before constructing this request, then hands the
 * ephemeral Google access token directly to the runner using the same exact
 * body capability and keyless native-identity contract as discovery.
 */

import { signRunnerCapabilityJwt, type NativeIdentityProvider } from "./runner-auth.js";
import { assertHostedScopes } from "./scopes.js";

export const ANALYTICS_PROPERTY_READ = "analytics_property_read" as const;
export const NATIVE_TOOL_CALL = "tool_call" as const;

export interface AnalyticsPropertyResult {
  name: string;
  displayName: string;
  timeZone: string;
  currencyCode: string;
}

export interface ToolExecutionRequest {
  tenantId: string;
  connectionId: string;
  service: string;
  resourceType: string;
  resourceId: string;
  accessToken: string;
  googleEmail: string;
  googleSubject: string;
  nativeTool?: {
    name: string;
    arguments: Record<string, unknown>;
    scopes: string[];
    allowedTools: string[];
    allowWrite: boolean;
    allowDestructive: boolean;
  };
}

export interface RunnerResponseDiagnostics {
  runnerStatus: number;
  runnerBodyJson: boolean;
}

export type ToolExecutionOutcome =
  | { status: "ok"; result: unknown }
  | { status: "unavailable" | "error"; detail: string | RunnerResponseDiagnostics };

export interface ToolRunner {
  execute(request: ToolExecutionRequest): Promise<ToolExecutionOutcome>;
}

export class UnavailableToolRunner implements ToolRunner {
  async execute(_request: ToolExecutionRequest): Promise<ToolExecutionOutcome> {
    return { status: "unavailable", detail: "runner_not_configured" };
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

interface WireAnalyticsPropertyResult {
  name: string;
  display_name: string;
  time_zone: string;
  currency_code: string;
}

function isWireAnalyticsPropertyResult(value: unknown): value is WireAnalyticsPropertyResult {
  if (!isPlainObject(value)) return false;
  const allowed = new Set(["name", "display_name", "time_zone", "currency_code"]);
  return (
    Object.keys(value).every((key) => allowed.has(key)) &&
    typeof value.name === "string" &&
    typeof value.display_name === "string" &&
    typeof value.time_zone === "string" &&
    typeof value.currency_code === "string"
  );
}

interface RunnerEnvelope {
  operation: string;
  ok: boolean;
  result: WireAnalyticsPropertyResult;
}

function isRunnerEnvelope(value: unknown): value is RunnerEnvelope {
  return (
    isPlainObject(value) &&
    value.operation === ANALYTICS_PROPERTY_READ &&
    value.ok === true &&
    isWireAnalyticsPropertyResult(value.result)
  );
}

/** Calls the fixed Cloud Run execute endpoint for one typed product read. */
export class RemoteToolRunner implements ToolRunner {
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

  async execute(request: ToolExecutionRequest): Promise<ToolExecutionOutcome> {
    if (request.nativeTool) {
      try {
        assertHostedScopes(request.nativeTool.scopes);
      } catch {
        return { status: "error", detail: "scope_blocked" };
      }
    }
    const deadlineSignal = AbortSignal.timeout(20_000);
    if (deadlineSignal.aborted) return { status: "error", detail: "runner_timeout" };

    let nativeIdToken: string;
    try {
      nativeIdToken = await this.identity.nativeIdToken(this.url.origin, deadlineSignal);
    } catch {
      return { status: "unavailable", detail: "operator_identity_unavailable" };
    }

    const resource = {
      service: request.service,
      resource_type: request.resourceType,
      resource_id: request.resourceId,
      enabled: true,
    };
    const operation = request.nativeTool ? NATIVE_TOOL_CALL : ANALYTICS_PROPERTY_READ;
    const body = {
      request_id: crypto.randomUUID(),
      tenant_id: request.tenantId,
      connection_id: request.connectionId,
      operation,
      google_email: request.googleEmail,
      google_subject: request.googleSubject,
      access_token: request.accessToken,
      ...(request.nativeTool
        ? {
            tool: {
              name: request.nativeTool.name,
              arguments: request.nativeTool.arguments,
              authorization: {
                service: request.service,
                allowed_tools: request.nativeTool.allowedTools,
                scopes: request.nativeTool.scopes,
                resource,
                allow_write: request.nativeTool.allowWrite,
                allow_destructive: request.nativeTool.allowDestructive,
              },
            },
          }
        : { resource }),
    };
    const exactBodyBytes = new TextEncoder().encode(JSON.stringify(body));
    const capability = await signRunnerCapabilityJwt(
      this.invocationToken,
      {
        tenantId: request.tenantId,
        connectionId: request.connectionId,
        operation,
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
        redirect: "manual",
        signal: deadlineSignal,
      });
    } catch (error) {
      return {
        status: "error",
        detail:
          "runner_unreachable:" +
          (error instanceof Error ? `${error.constructor.name}: ${error.message}` : String(error)),
      };
    }
    let wire: unknown;
    let runnerBodyJson = true;
    try {
      wire = await response.json();
    } catch {
      runnerBodyJson = false;
    }
    if (response.status >= 300 && response.status < 400) {
      return { status: "error", detail: "runner_redirect_blocked" };
    }
    if (response.status < 200 || response.status >= 300) {
      return {
        status: "error",
        detail: { runnerStatus: response.status, runnerBodyJson },
      };
    }
    if (!runnerBodyJson) {
      return {
        status: "error",
        detail: { runnerStatus: response.status, runnerBodyJson },
      };
    }
    if (request.nativeTool) {
      if (
        !isPlainObject(wire) ||
        wire.operation !== NATIVE_TOOL_CALL ||
        wire.ok !== true ||
        !(isPlainObject(wire.result) || typeof wire.result === "string")
      ) {
        return { status: "error", detail: "runner_invalid_response" };
      }
      return { status: "ok", result: wire.result };
    }
    if (!isRunnerEnvelope(wire)) {
      return { status: "error", detail: "runner_invalid_response" };
    }

    const raw = wire.result;
    return {
      status: "ok",
      result: {
        name: raw.name,
        displayName: raw.display_name,
        timeZone: raw.time_zone,
        currencyCode: raw.currency_code,
      },
    };
  }
}
