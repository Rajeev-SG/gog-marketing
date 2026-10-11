import catalog from "./tool-catalog.generated.json";
import { HostedRepository } from "../../state/src/repository.js";
import { ConnectError, type ConnectSession } from "./connect.js";

export const EXECUTABLE_TOOLS = catalog;

export function executableTool(name: string) {
  return EXECUTABLE_TOOLS.find((entry) => entry.name === name);
}

export async function toolPermissionView(
  repo: HostedRepository,
  session: ConnectSession,
  connectionId: string,
) {
  if (!(await repo.getConnection(session.tenantId, connectionId)))
    throw new ConnectError("connection_not_found", 404);
  const grants = await repo.listToolGrants(session.tenantId, connectionId);
  return {
    connectionId,
    tools: EXECUTABLE_TOOLS.map((entry) => {
      const grant = grants.find(
        (candidate) => candidate.toolName === entry.name && candidate.service === entry.service,
      );
      return {
        name: entry.name,
        service: entry.service,
        description: entry.tool.description,
        write: entry.write,
        enabled: grant?.enabled ?? false,
        allowDestructive: grant?.allowDestructive ?? false,
      };
    }),
  };
}

export async function saveToolPermissions(
  repo: HostedRepository,
  session: ConnectSession,
  connectionId: string,
  raw: unknown,
) {
  if (!(await repo.getConnection(session.tenantId, connectionId)))
    throw new ConnectError("connection_not_found", 404);
  if (!Array.isArray(raw) || raw.length > 128) throw new ConnectError("invalid_request", 400);
  const seen = new Set<string>();
  const updates = raw.map((value: unknown) => {
    if (!value || typeof value !== "object" || Array.isArray(value))
      throw new ConnectError("invalid_request", 400);
    const input = value as Record<string, unknown>;
    if (
      Object.keys(input).some(
        (key) => !["toolName", "enabled", "allowDestructive"].includes(key),
      ) ||
      typeof input.toolName !== "string" ||
      typeof input.enabled !== "boolean" ||
      typeof input.allowDestructive !== "boolean"
    )
      throw new ConnectError("invalid_request", 400);
    const tool = executableTool(input.toolName);
    if (
      !tool ||
      seen.has(input.toolName) ||
      (!tool.write && input.allowDestructive) ||
      (!input.enabled && input.allowDestructive)
    )
      throw new ConnectError("invalid_request", 400);
    seen.add(input.toolName);
    return {
      toolName: tool.name,
      service: tool.service,
      enabled: input.enabled,
      allowDestructive: input.allowDestructive,
      allowBillable: false,
    };
  });
  await repo.saveToolGrants(session.tenantId, connectionId, updates);
  await repo.appendAudit({
    id: crypto.randomUUID(),
    tenantId: session.tenantId,
    connectionId,
    actorClerkUserId: session.userId,
    action: "google.tools.save",
    result: "allow",
    detailJson: JSON.stringify({ operation: "save", count: updates.length, error: "none" }),
    latencyMs: null,
  });
  return toolPermissionView(repo, session, connectionId);
}
