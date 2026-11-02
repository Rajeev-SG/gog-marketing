import type { HostedRepository } from "../../state/src/repository.js";
import type { AuditEvent } from "../../state/src/types.js";
import { HOSTED_SERVICES } from "./scopes.js";

/**
 * Activity log (N01): a plain-language view of the tenant's own audit events.
 * Only allow-listed, already-sanitised metadata leaves the server; raw
 * detail JSON, actor IDs and latency never do.
 */
type ActivityTone = "ok" | "blocked" | "failed" | "info";
export interface ActivityItem {
  id: string;
  at: string;
  connectionId: string;
  /** "tool": an AI app used gog-marketing. "setting": you changed access. */
  kind: "tool" | "setting";
  tone: ActivityTone;
  title: string;
  detail: string;
}

const ERROR_TEXT: Readonly<Record<string, string>> = {
  permission_denied: "the tool or asset isn’t enabled for this account",
  rate_limited: "too many requests in a short time",
  needs_reconnect: "Google asked you to reconnect this account",
  internal_error: "an unexpected error",
  invalid_request: "the request was invalid",
  quota_exceeded: "the daily limit was reached",
};

function detail(json: string): Record<string, string | number> {
  try {
    const parsed = JSON.parse(json) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const out: Record<string, string | number> = {};
    for (const key of ["operation", "service", "resource", "error", "count"]) {
      const value = (parsed as Record<string, unknown>)[key];
      if (typeof value === "string" && value.length <= 200) out[key] = value;
      if (typeof value === "number" && Number.isFinite(value)) out[key] = value;
    }
    return out;
  } catch {
    return {};
  }
}

const serviceName = (key: unknown) =>
  typeof key === "string" && key ? (HOSTED_SERVICES[key]?.label ?? key) : "";
const toolName = (operation: unknown, service: unknown) => {
  if (typeof operation !== "string" || !operation) return "A tool";
  let short = operation;
  if (typeof service === "string" && short.startsWith(service + "_"))
    short = short.slice(service.length + 1);
  const words = short.split("_").filter(Boolean).join(" ");
  const label = serviceName(service);
  return (label ? `${label}: ` : "") + words.charAt(0).toUpperCase() + words.slice(1);
};
const reason = (error: unknown) =>
  typeof error === "string" && error !== "none" ? (ERROR_TEXT[error] ?? "an error") : "";
const count = (value: unknown, one: string, many: string) =>
  typeof value === "number" ? `${value} ${value === 1 ? one : many}` : "";

export function describeAuditEvent(event: AuditEvent): ActivityItem {
  const d = detail(event.detailJson);
  const base = {
    id: event.id,
    at: event.createdAt,
    connectionId: event.connectionId,
    kind: event.action.startsWith("mcp.") ? ("tool" as const) : ("setting" as const),
  };
  const why = reason(d.error);
  switch (event.action) {
    case "mcp.tools.call": {
      const tool = toolName(d.operation, d.service);
      const on = typeof d.resource === "string" ? `On ${d.resource}` : "";
      if (event.result === "allow")
        return { ...base, tone: "ok", title: `${tool} ran`, detail: on };
      if (event.result === "deny")
        return {
          ...base,
          tone: "blocked",
          title: `${tool} was blocked`,
          detail: [on, why && `Blocked because ${why}.`].filter(Boolean).join(" · "),
        };
      return {
        ...base,
        tone: "failed",
        title: `${tool} failed`,
        detail: [on, why && `Failed with ${why}.`].filter(Boolean).join(" · "),
      };
    }
    case "mcp.request.rate_limited":
      return {
        ...base,
        tone: "blocked",
        title: "AI requests were paused briefly",
        detail: "Too many requests arrived in one minute. They resume automatically.",
      };
    case "google.tools.save":
      return {
        ...base,
        tone: "info",
        title: "AI tool permissions saved",
        detail: count(d.count, "tool reviewed", "tools reviewed"),
      };
    case "google.grants.save":
      return {
        ...base,
        tone: "info",
        title: "Data access saved",
        detail: count(d.count, "asset changed", "assets changed"),
      };
    case "google.picker.complete":
      return {
        ...base,
        tone: "info",
        title: "Google files chosen",
        detail: count(d.count, "file added", "files added") + " (not yet enabled)",
      };
    case "google.connect.start":
      return { ...base, tone: "info", title: "Google sign-in started", detail: "" };
    case "google.connect.complete":
      return { ...base, tone: "ok", title: "Google account connected", detail: "" };
    case "google.connect.consent_denied":
      return {
        ...base,
        tone: "blocked",
        title: "Google sign-in was cancelled",
        detail: "Existing access was not changed.",
      };
    case "google.connect.failure":
      return {
        ...base,
        tone: "failed",
        title: "Google connection failed",
        detail: why ? `Failed because ${why}.` : "",
      };
    case "google.connect.refresh":
      return event.result === "allow"
        ? { ...base, tone: "ok", title: "Connection refreshed", detail: "" }
        : {
            ...base,
            tone: "failed",
            title: "Connection refresh failed",
            detail: why ? `Failed because ${why}.` : "",
          };
    case "google.discovery":
      return event.result === "error"
        ? {
            ...base,
            tone: "failed",
            title: "Asset check failed",
            detail: "Existing selections were kept.",
          }
        : { ...base, tone: "info", title: "Assets checked", detail: "" };
    case "google.disconnect":
      return { ...base, tone: "info", title: "Google account disconnected", detail: "" };
    default:
      return { ...base, tone: "info", title: "Workspace activity", detail: "" };
  }
}

export async function activityView(repo: HostedRepository, tenantId: string) {
  const events = await repo.listAudit(tenantId, 200);
  return { events: events.map(describeAuditEvent) };
}
