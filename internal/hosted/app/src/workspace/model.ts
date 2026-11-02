/**
 * Workspace view model: the pure rules behind Home, Google data and Access.
 * No DOM or React here, so every rule is unit-tested directly.
 */
import { HOSTED_SERVICES } from "../scopes.js";

export interface Resource {
  service: string;
  serviceLabel?: string;
  resourceId: string;
  displayName?: string;
  resourceType?: string;
  parent?: string;
  enabled: boolean;
}
export interface Connection {
  id: string;
  email?: string;
  displayName?: string;
  status: string;
  discovery: { status?: string };
  services?: string[];
  resources?: Resource[];
}
export interface Tool {
  name: string;
  service: string;
  description: string;
  write: boolean;
  enabled: boolean;
  allowDestructive: boolean;
}
export interface ToolChoice {
  toolName: string;
  enabled: boolean;
  allowDestructive: boolean;
}
export type ToolFilter = "all" | "reads" | "changes";
export type StatusKind = "ok" | "needs_attention" | "unavailable" | "disconnected";

const MARKETING = new Set([
  "analytics",
  "tagmanager",
  "searchconsole",
  "bigquery",
  "adsense",
  "youtube",
]);

export function plural(count: number, one: string, many: string): string {
  return count === 1 ? one : many;
}
export function category(service: string): "Marketing" | "Workspace" {
  return MARKETING.has(service) ? "Marketing" : "Workspace";
}
export function serviceLabel(service: string): string {
  return HOSTED_SERVICES[service]?.label ?? service;
}
/** Connections list services by label; map them back to catalogue keys. */
export function serviceKey(label: string): string {
  return Object.keys(HOSTED_SERVICES).find((key) => HOSTED_SERVICES[key].label === label) ?? "";
}
export function accountName(connection: Connection | undefined): string {
  return connection?.email || connection?.displayName || "this Google account";
}
export function discoveryDelayed(connection: Connection): boolean {
  return connection.discovery?.status === "error" || connection.discovery?.status === "unavailable";
}
export function safeStatus(connection: Connection): [StatusKind, string] {
  if (connection.status === "needs_reconnect") return ["needs_attention", "Needs attention"];
  if (connection.status === "revoked") return ["unavailable", "Access paused"];
  if (discoveryDelayed(connection)) return ["unavailable", "Check delayed"];
  if (connection.discovery?.status === "empty") return ["disconnected", "Ready"];
  return ["ok", "Healthy"];
}
export const resourceKey = (resource: Pick<Resource, "service" | "resourceId">) =>
  resource.service + "\u0000" + resource.resourceId;

export function enabledCount(connection: Connection): number {
  return (connection.resources ?? []).filter((resource) => resource.enabled).length;
}

export interface WorkspaceSummary {
  accounts: number;
  accountsLabel: string;
  enabledAssets: number;
  totalAssets: number;
  services: number;
  servicesLabel: string;
  health: string;
  healthDetail: string;
  attention: boolean;
}
export function workspaceSummary(connections: Connection[]): WorkspaceSummary {
  const active = connections.filter((item) => item.status === "active").length;
  const needsAttention = connections.some((item) => item.status !== "active");
  const delayed = connections.some(discoveryDelayed);
  const services = new Set<string>();
  for (const connection of connections) {
    for (const resource of connection.resources ?? []) services.add(resource.service);
    for (const name of connection.services ?? []) {
      const key = serviceKey(name);
      if (key) services.add(key);
    }
  }
  return {
    accounts: connections.length,
    accountsLabel:
      plural(connections.length, "Google account", "Google accounts") +
      (active < connections.length ? ` · ${connections.length - active} need attention` : ""),
    enabledAssets: connections.reduce((total, item) => total + enabledCount(item), 0),
    totalAssets: connections.reduce((total, item) => total + (item.resources ?? []).length, 0),
    services: services.size,
    servicesLabel: plural(services.size, "service connected", "services connected"),
    health: needsAttention ? "Needs attention" : delayed ? "Some checks delayed" : "All healthy",
    healthDetail: needsAttention
      ? "Review the highlighted account"
      : delayed
        ? "Existing access remains available"
        : "No issues detected",
    attention: needsAttention,
  };
}

export interface ServiceOverviewRow {
  service: string;
  label: string;
  category: string;
  total: number;
  enabled: number;
  accounts: number;
  status: "attention" | "delayed" | "empty" | "ok";
  /** First account with discovered assets for this service, for the row link. */
  ownerId: string | null;
}
/** One row per granted service, counting every account that granted it (D02). */
export function serviceOverview(connections: Connection[]): ServiceOverviewRow[] {
  const rows = new Map<
    string,
    ServiceOverviewRow & { accountIds: Set<string>; delayed: boolean; attention: boolean }
  >();
  const entry = (key: string, label?: string) => {
    let row = rows.get(key);
    if (!row) {
      row = {
        service: key,
        label: label || serviceLabel(key),
        category: category(key),
        total: 0,
        enabled: 0,
        accounts: 0,
        status: "ok",
        ownerId: null,
        accountIds: new Set(),
        delayed: false,
        attention: false,
      };
      rows.set(key, row);
    }
    return row;
  };
  for (const connection of connections) {
    for (const resource of connection.resources ?? []) {
      const row = entry(resource.service, resource.serviceLabel);
      row.total += 1;
      if (resource.enabled) row.enabled += 1;
      row.ownerId ??= connection.id;
    }
    const granted = new Set((connection.resources ?? []).map((resource) => resource.service));
    for (const name of connection.services ?? []) {
      const key = serviceKey(name);
      if (key) granted.add(key);
    }
    for (const key of granted) {
      const row = entry(key);
      row.accountIds.add(connection.id);
      if (discoveryDelayed(connection)) row.delayed = true;
      if (connection.status !== "active") row.attention = true;
    }
  }
  return [...rows.values()]
    .map(({ accountIds, delayed, attention, ...row }) => ({
      ...row,
      accounts: accountIds.size,
      status: (attention
        ? "attention"
        : delayed
          ? "delayed"
          : row.total === 0
            ? "empty"
            : "ok") as ServiceOverviewRow["status"],
    }))
    .sort((a, b) => a.category.localeCompare(b.category) || a.label.localeCompare(b.label));
}

export interface ServiceGroup {
  service: string;
  label: string;
  resources: Resource[];
}
export function serviceGroups(connection: Connection | undefined): ServiceGroup[] {
  const groups = new Map<string, ServiceGroup>();
  for (const resource of connection?.resources ?? []) {
    let group = groups.get(resource.service);
    if (!group) {
      group = {
        service: resource.service,
        label: resource.serviceLabel || serviceLabel(resource.service),
        resources: [],
      };
      groups.set(resource.service, group);
    }
    group.resources.push(resource);
  }
  return [...groups.values()].sort(
    (a, b) =>
      category(a.service).localeCompare(category(b.service)) || a.label.localeCompare(b.label),
  );
}
export function matchesAsset(resource: Resource, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [resource.displayName, resource.parent, resource.resourceId]
    .join(" ")
    .toLowerCase()
    .includes(needle);
}

/** Asset grants that differ from what is saved, in the save payload shape. */
export function changedGrants(
  connection: Connection | undefined,
  draft: ReadonlyMap<string, boolean>,
) {
  return (connection?.resources ?? [])
    .filter(
      (resource) => (draft.get(resourceKey(resource)) ?? resource.enabled) !== resource.enabled,
    )
    .map((resource) => ({
      service: resource.service,
      resourceId: resource.resourceId,
      enabled: draft.get(resourceKey(resource)) === true,
    }));
}
export function pendingSummary(assetChanges: number, toolsDirty: boolean): string {
  const parts: string[] = [];
  if (assetChanges)
    parts.push(`${assetChanges} asset ${plural(assetChanges, "change", "changes")}`);
  if (toolsDirty) parts.push("tool permission changes");
  return parts.length ? "Unsaved: " + parts.join(" and ") : "No unsaved changes";
}

export interface SetupStep {
  done: { account: boolean; data: boolean; tools: boolean };
  copy: string;
  action: { label: string; href: string };
  complete: boolean;
}
export function setupStep(connection: Connection | undefined, toolsEnabled: boolean): SetupStep {
  const active = Boolean(connection && connection.status === "active");
  const data = active && enabledCount(connection!) > 0;
  const tools = data && toolsEnabled;
  const who = connection ? accountName(connection) : "";
  const done = { account: active, data, tools };
  if (!connection)
    return {
      done,
      complete: false,
      copy: "Start by connecting a Google account. Nothing is enabled automatically.",
      action: { label: "Connect Google account", href: "/access" },
    };
  if (!active)
    return {
      done,
      complete: false,
      copy: `Review ${who} before continuing. Your saved selections are preserved.`,
      action: { label: "Review account", href: "/data" },
    };
  if (!data)
    return {
      done,
      complete: false,
      copy: `Choose only the files or resources you want AI to access for ${who}.`,
      action: { label: "Choose data", href: "/data" },
    };
  if (!tools)
    return {
      done,
      complete: false,
      copy: "Your selected data is saved. Now choose exact AI tools; start with reads and opt into changes only when needed.",
      action: { label: "Choose AI tools", href: "/data#tools" },
    };
  return {
    done,
    complete: true,
    copy: `Data and tools are enabled for ${who}. Every Google write still needs review.`,
    action: { label: "Open chat", href: "/chat" },
  };
}

export function readableToolLabel(tool: Pick<Tool, "name" | "service">): string {
  let short = tool.name;
  if (short.startsWith(tool.service + "_")) short = short.slice(tool.service.length + 1);
  const words = short
    .split("_")
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1));
  return `${serviceLabel(tool.service)} · ${words.join(" ")}`;
}
export function filteredToolCatalog(
  catalog: readonly Tool[],
  query: string,
  filter: ToolFilter,
): Tool[] {
  const needle = (query || "").trim().toLowerCase();
  return catalog.filter((tool) => {
    if (filter === "reads" && tool.write) return false;
    if (filter === "changes" && !tool.write) return false;
    if (!needle) return true;
    return [readableToolLabel(tool), tool.name, tool.service, tool.description]
      .join(" ")
      .toLowerCase()
      .includes(needle);
  });
}
/** Enables only the visible read-only tools; write and destructive choices never change. */
export function enableVisibleReadTools(
  choices: Map<string, ToolChoice>,
  catalog: readonly Tool[],
  query: string,
  filter: ToolFilter,
): number {
  let changed = 0;
  for (const tool of filteredToolCatalog(catalog, query, filter)) {
    const choice = choices.get(tool.name);
    if (!choice || tool.write || choice.enabled) continue;
    choice.enabled = true;
    changed += 1;
  }
  return changed;
}
export function toolGroups(tools: readonly Tool[]): Array<{ service: string; tools: Tool[] }> {
  const groups = new Map<string, Tool[]>();
  for (const tool of tools) {
    if (!groups.has(tool.service)) groups.set(tool.service, []);
    groups.get(tool.service)!.push(tool);
  }
  return [...groups.entries()]
    .map(([service, items]) => ({ service, tools: items }))
    .sort(
      (a, b) =>
        category(a.service).localeCompare(category(b.service)) ||
        serviceLabel(a.service).localeCompare(serviceLabel(b.service)),
    );
}
/** The exact save payload: toolName, enabled and allowDestructive only. */
export function toolGrantPayload(choices: ReadonlyMap<string, ToolChoice>): ToolChoice[] {
  return [...choices.values()].map(({ toolName, enabled, allowDestructive }) => ({
    toolName,
    enabled,
    allowDestructive,
  }));
}

const CONNECT_OUTCOMES: Readonly<Record<string, readonly [string, "info" | "error" | ""]>> = {
  connected: ["Google account connected.", "info"],
  consent_denied: ["Google consent was denied. Your previous access was not changed.", ""],
  operator_config_missing: ["Google connection is not configured yet.", ""],
  google_unavailable: ["Google is temporarily unavailable. Please try again.", ""],
  google_api_error: ["Google could not complete the connection. Please try again.", ""],
  identity_conflict: ["Already connected — this Google account is already available.", "info"],
  reconnect_identity_mismatch: [
    "Choose the original Google account for this connection, or add this account separately.",
    "error",
  ],
  state_expired: ["The connection attempt expired. Please start again.", ""],
  needs_reconnect: ["This Google account needs to be reconnected.", "error"],
  state_replayed: ["This connection attempt was already completed.", "info"],
};
export function connectOutcome(outcome: string): readonly [string, "info" | "error" | ""] {
  return CONNECT_OUTCOMES[outcome] ?? ["The Google connection attempt could not be completed.", ""];
}

export interface ConnectRequest {
  services: string[];
  writeServices: string[];
  connectionId?: string;
}
/**
 * Starts Google consent. Resolves to the authorization URL, or throws an Error
 * whose message already carries retry guidance. Aborts after `timeoutMs`.
 */
export async function requestConnection(
  body: ConnectRequest,
  fetcher: typeof fetch = fetch,
  timeoutMs = 20_000,
): Promise<string> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const guidance = " Check your network, then select the button again.";
  try {
    const response = await fetcher("/api/google/connect", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    let result: { authorizationUrl?: string; error?: string } = {};
    try {
      result = await response.json();
    } catch {
      if (controller.signal.aborted) throw new Error("The Google sign-in request timed out.");
    }
    if (!response.ok || !result.authorizationUrl)
      throw new Error(result.error || "Could not start Google connection.");
    return result.authorizationUrl;
  } catch (error) {
    // Every failure ends with the same retry guidance, including network errors.
    let reason = controller.signal.aborted
      ? "The Google sign-in request timed out."
      : (error instanceof Error && error.message) || "Could not start Google connection.";
    if (!reason.endsWith(".")) reason += ".";
    throw new Error(reason + guidance, { cause: error });
  } finally {
    clearTimeout(timer);
  }
}
