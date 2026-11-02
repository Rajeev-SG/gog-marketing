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
function category(service: string): "Marketing" | "Workspace" {
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
      action: { label: "Connect Google account", href: "/settings/accounts#connect" },
    };
  if (!active)
    return {
      done,
      complete: false,
      copy: `Review ${who} before continuing. Your saved selections are preserved.`,
      action: { label: "Review account", href: "/settings/accounts" },
    };
  if (!data)
    return {
      done,
      complete: false,
      copy: `Choose only the files or resources you want AI to access for ${who}.`,
      action: { label: "Choose data", href: "/settings/data" },
    };
  if (!tools)
    return {
      done,
      complete: false,
      copy: "Your selected data is saved. Now choose exact AI tools; start with reads and opt into changes only when needed.",
      action: { label: "Choose AI tools", href: "/settings/tools" },
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
/** The tool's action without its service, for lists already grouped by service. */
export function toolActionLabel(tool: Pick<Tool, "name" | "service">): string {
  return readableToolLabel(tool).split(" · ").slice(1).join(" · ") || tool.name;
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

// ---- Settings: what is shared, what is enabled, what needs attention ---------------

/** Catalogue keys of every service this account shares with gog-marketing. */
function sharedServices(connection: Connection | undefined): Set<string> {
  const shared = new Set<string>();
  for (const name of connection?.services ?? []) {
    const key = serviceKey(name);
    if (key) shared.add(key);
  }
  for (const resource of connection?.resources ?? []) shared.add(resource.service);
  return shared;
}

export interface ToolSummary {
  /** Tools whose Google service this account shares. */
  available: number;
  enabled: number;
  /** Enabled tools that can change Google data. */
  changes: number;
  /** Enabled tools also allowed to delete or replace data. */
  destructive: number;
  /** Enabled tools that cannot run because their service isn't shared. */
  stranded: number;
}
export function toolSummary(connection: Connection, tools: readonly Tool[]): ToolSummary {
  const shared = sharedServices(connection);
  const summary: ToolSummary = {
    available: 0,
    enabled: 0,
    changes: 0,
    destructive: 0,
    stranded: 0,
  };
  for (const tool of tools) {
    const isShared = shared.has(tool.service);
    if (isShared) summary.available += 1;
    if (!tool.enabled) continue;
    if (!isShared) {
      summary.stranded += 1;
      continue;
    }
    summary.enabled += 1;
    if (tool.write) summary.changes += 1;
    if (tool.allowDestructive) summary.destructive += 1;
  }
  return summary;
}

/** Tool groups shown for an account: shared services, plus any with stranded grants. */
export function visibleToolGroups(
  connection: Connection | undefined,
  tools: readonly Tool[],
  choices: ReadonlyMap<string, ToolChoice>,
  showUnshared: boolean,
) {
  const shared = sharedServices(connection);
  return toolGroups(tools)
    .map((group) => ({ ...group, shared: shared.has(group.service) }))
    .filter(
      (group) =>
        showUnshared || group.shared || group.tools.some((tool) => choices.get(tool.name)?.enabled),
    );
}
export function hiddenToolCount(connection: Connection | undefined, tools: readonly Tool[]) {
  const shared = sharedServices(connection);
  const hidden = tools.filter((tool) => !shared.has(tool.service));
  return { tools: hidden.length, services: new Set(hidden.map((tool) => tool.service)).size };
}

type SettingsSection = "accounts" | "data" | "tools" | "activity" | "connect-ai";
export interface AttentionItem {
  id: string;
  /** "action": something is broken or blocked. "todo": setup is incomplete. */
  level: "action" | "todo";
  section: SettingsSection;
  title: string;
  detail: string;
  action: { label: string; href?: string; discover?: string };
}
const enc = encodeURIComponent;

/**
 * Everything in Settings that needs the user, most urgent first. Healthy
 * configuration produces nothing: status is shown only when it needs action.
 */
export function attentionItems(
  connections: readonly Connection[],
  tools: ReadonlyMap<string, readonly Tool[]>,
  recentBlocked = 0,
): AttentionItem[] {
  const items: AttentionItem[] = [];
  if (!connections.length)
    items.push({
      id: "connect",
      level: "todo",
      section: "accounts",
      title: "Connect a Google account",
      detail: "AI tools have no Google data until you connect an account and choose what to share.",
      action: { label: "Connect Google account", href: "/settings/accounts#connect" },
    });
  for (const connection of connections) {
    const who = accountName(connection);
    if (connection.status === "needs_reconnect" || connection.status === "revoked") {
      items.push({
        id: `reconnect:${connection.id}`,
        level: "action",
        section: "accounts",
        title: `Reconnect ${who}`,
        detail:
          connection.status === "revoked"
            ? "Google access for this account is paused. AI tools can’t use it until you reconnect."
            : "Google needs you to sign in again. Your saved choices are kept.",
        action: {
          label: "Reconnect Google",
          href: `/settings/accounts?update=${enc(connection.id)}#connect`,
        },
      });
      continue;
    }
    if (discoveryDelayed(connection))
      items.push({
        id: `discover:${connection.id}`,
        level: "action",
        section: "accounts",
        title: `Some services couldn’t be checked for ${who}`,
        detail:
          "Existing selections still work. New assets may be missing until the check succeeds.",
        action: { label: "Check again", discover: connection.id },
      });
    const resources = connection.resources ?? [];
    const assets = enabledCount(connection);
    if (resources.length > 0 && assets === 0)
      items.push({
        id: `data:${connection.id}`,
        level: "todo",
        section: "data",
        title: `Choose data for ${who}`,
        detail: `${resources.length} ${plural(resources.length, "asset was", "assets were")} found, but none is enabled, so AI tools can’t use this account yet.`,
        action: { label: "Choose data", href: `/settings/data?account=${enc(connection.id)}` },
      });
    const catalog = tools.get(connection.id);
    if (!catalog) continue;
    const summary = toolSummary(connection, catalog);
    if (assets > 0 && summary.enabled === 0)
      items.push({
        id: `tools:${connection.id}`,
        level: "todo",
        section: "tools",
        title: `Choose AI tools for ${who}`,
        detail: "Data is enabled, but no AI tool may use it yet.",
        action: { label: "Choose AI tools", href: `/settings/tools?account=${enc(connection.id)}` },
      });
    if (summary.stranded > 0)
      items.push({
        id: `stranded:${connection.id}`,
        level: "todo",
        section: "tools",
        title: `${summary.stranded} enabled ${plural(summary.stranded, "tool", "tools")} can’t run for ${who}`,
        detail:
          "Their Google service isn’t shared by this account. Share the service, or turn these tools off.",
        action: { label: "Review tools", href: `/settings/tools?account=${enc(connection.id)}` },
      });
  }
  if (recentBlocked > 0)
    items.push({
      id: "blocked",
      level: "todo",
      section: "activity",
      title: `${recentBlocked} AI ${plural(recentBlocked, "request", "requests")} blocked or failed in the last 7 days`,
      detail:
        "Usually an AI app asked for data or a tool you haven’t enabled. Check whether it needs access.",
      action: { label: "View problems", href: "/settings/activity?show=problems" },
    });
  return items.sort((a, b) => (a.level === b.level ? 0 : a.level === "action" ? -1 : 1));
}

export function attentionBySection(items: readonly AttentionItem[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const item of items) counts[item.section] = (counts[item.section] ?? 0) + 1;
  return counts;
}

export interface ActivityItem {
  id: string;
  at: string;
  connectionId: string;
  kind: "tool" | "setting";
  tone: "ok" | "blocked" | "failed" | "info";
  title: string;
  detail: string;
}
const WEEK = 7 * 24 * 60 * 60 * 1000;
export function recentBlockedCount(events: readonly ActivityItem[], now = Date.now()): number {
  return events.filter(
    (event) =>
      (event.tone === "blocked" || event.tone === "failed") && now - Date.parse(event.at) <= WEEK,
  ).length;
}
