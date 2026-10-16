/** In-process MCP bridge: the same catalogue, grants, quota, audit and runner as /mcp.
 * No OAuth client, self-issued bearer token, or browser-supplied tenant authority.
 */
import { jsonSchema, tool, type ToolSet } from "ai";
import type { AuthSession } from "../auth.js";
import { handleToolCall, policyTools, type McpEnv } from "../mcp.js";
import { executableTool } from "../tool-permissions.js";
import { HostedRepository } from "../../../state/src/repository.js";

// A bounded starter surface, not CLI parity. These are existing fixed runner mappings.
export const CHAT_GOOGLE_TOOLS = new Set([
  "drive_get",
  "docs_get",
  "docs_write",
  "sheets_metadata",
  "sheets_read_range",
  "sheets_update_range",
  "sheets_append_range",
  "calendar_events",
  "calendar_event_get",
  "calendar_event_create",
  "calendar_event_update",
  "analytics_properties_get",
  "analytics_report",
  "analytics_datastreams_list",
  "searchconsole_sites_list",
  "searchconsole_query",
  "searchconsole_inspect",
  "bigquery_datasets_list",
  "bigquery_tables_list",
  "bigquery_table_get",
  "bigquery_table_rows",
  "tagmanager_containers_list",
  "tagmanager_workspaces_list",
  "tagmanager_tags_list",
  "tagmanager_tags_get",
  "slides_list_slides",
  "slides_update_notes",
  "tasks_lists",
  "tasks_list",
  "tasks_get",
  "tasks_add",
  "tasks_update",
  "tasks_done",
  "tasks_delete",
  "tasks_lists_create",
  "contacts_list",
  "contacts_search",
  "contacts_get",
  "contacts_create",
  "contacts_update",
  "drivelabels_list",
  "drivelabels_get",
  "groups_list",
  "groups_members",
  "meet_space_get",
  "meet_space_create",
  "adsense_accounts_list",
  "adsense_accounts_get",
  "sites_get",
  "sites_url",
  "forms_get",
  "forms_responses_list",
  "appscript_get",
  "appscript_content",
  "chat_spaces_list",
  "chat_messages_send",
  "classroom_courses_list",
  "classroom_courses_get",
  "youtube_playlists_list",
  "youtube_playlists_create",
  "people_me",
  "photos_list",
  "photos_get",
  "photos_search",
]);
const PREFIX = "google-approval:";
const TTL = 10 * 60_000;
interface Approval {
  name: string;
  input: string;
  tenantId: string;
  userId: string;
  expiresAt: number;
  state: "pending" | "approved" | "denied" | "consumed" | "expired";
}
// Object-key order is not authority; all other input differences invalidate approval.
function canonical(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value !== null && typeof value === "object")
    return (
      "{" +
      Object.entries(value)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, item]) => JSON.stringify(key) + ":" + canonical(item))
        .join(",") +
      "}"
    );
  return JSON.stringify(value);
}

/** Durable, one-use approval independently gates writes even if browser history is forged. */
export class GoogleApprovals {
  constructor(private storage: DurableObjectStorage) {}
  async request(
    id: string,
    name: string,
    input: unknown,
    session: AuthSession,
  ): Promise<"user-approval" | "approved" | "denied"> {
    if (!id || id.length > 256) throw new Error("Invalid tool call.");
    const entries = await this.storage.list<Approval>({ prefix: PREFIX });
    // Retain compact tombstones: an old call ID must never acquire a fresh approval.
    for (const [key, value] of entries) {
      if (value.expiresAt <= Date.now() && value.state !== "expired")
        await this.storage.put(key, { ...value, name: "", input: "", state: "expired" });
    }
    const active = [...entries.values()].filter((value) => value.expiresAt > Date.now()).length;
    if (active >= 100 && !entries.has(PREFIX + id)) throw new Error("Too many pending actions.");
    return this.storage.transaction(async (tx) => {
      const existing = await tx.get<Approval>(PREFIX + id);
      if (existing) {
        if (existing.expiresAt <= Date.now() || existing.state === "expired") return "denied";
        if (
          existing.name !== name ||
          existing.input !== canonical(input) ||
          existing.userId !== session.userId ||
          existing.tenantId !== session.tenant.id
        )
          throw new Error("Tool call arguments changed.");
        return existing.state === "pending"
          ? "user-approval"
          : existing.state === "approved"
            ? "approved"
            : "denied";
      }
      await tx.put<Approval>(PREFIX + id, {
        name,
        input: canonical(input),
        tenantId: session.tenant.id,
        userId: session.userId,
        expiresAt: Date.now() + TTL,
        state: "pending",
      });
      return "user-approval";
    });
  }
  async respond(id: string, approved: boolean, session: AuthSession): Promise<boolean> {
    return this.storage.transaction(async (tx) => {
      const record = await tx.get<Approval>(PREFIX + id);
      if (
        !record ||
        record.state !== "pending" ||
        record.expiresAt <= Date.now() ||
        record.userId !== session.userId ||
        record.tenantId !== session.tenant.id
      )
        return false;
      await tx.put(PREFIX + id, { ...record, state: approved ? "approved" : "denied" });
      return true;
    });
  }
  async consume(id: string, name: string, input: unknown, session: AuthSession): Promise<boolean> {
    return this.storage.transaction(async (tx) => {
      const record = await tx.get<Approval>(PREFIX + id);
      if (
        !record ||
        record.state !== "approved" ||
        record.expiresAt <= Date.now() ||
        record.userId !== session.userId ||
        record.tenantId !== session.tenant.id ||
        record.name !== name ||
        record.input !== canonical(input)
      )
        return false;
      // Mark before dispatch. An interrupted/unknown write is never blindly retried.
      await tx.put(PREFIX + id, { ...record, state: "consumed" });
      return true;
    });
  }
}

export async function googleTools(
  env: McpEnv,
  repo: HostedRepository,
  session: AuthSession,
  approvals: GoogleApprovals,
  authorize: () => Promise<void>,
): Promise<ToolSet> {
  const available = (await policyTools(repo, session.tenant)).filter((t) =>
    CHAT_GOOGLE_TOOLS.has(t.name),
  );
  const tools: ToolSet = {
    google_access: tool({
      description:
        "List this user's enabled Google accounts, tools and selected resource IDs. Use before choosing a Google account/resource. Does not grant access.",
      inputSchema: jsonSchema<Record<string, unknown>>({
        type: "object",
        properties: {},
        additionalProperties: false,
      }),
      execute: async () => {
        await authorize();
        const current = (await policyTools(repo, session.tenant)).filter((t) =>
          CHAT_GOOGLE_TOOLS.has(t.name),
        );
        const accounts = [];
        for (const connection of await repo.listConnections(session.tenant.id)) {
          if (connection.status !== "active") continue;
          const grants = await repo.listToolGrants(session.tenant.id, connection.id);
          const names = grants
            .filter((g) => g.enabled && current.some((t) => t.name === g.toolName))
            .map((g) => g.toolName);
          if (!names.length) continue;
          const resources = (await repo.listResourceGrants(session.tenant.id, connection.id))
            .filter(
              (g) => g.enabled && names.some((name) => executableTool(name)?.service === g.service),
            )
            .map((g) => ({
              service: g.service,
              type: g.resourceType,
              id: g.resourceId,
              name: g.displayName,
            }));
          accounts.push({
            connectionId: connection.id,
            email: connection.email,
            tools: names,
            resources,
          });
        }
        return {
          accounts,
          note: "Only enabled tools/resources may be used. Writes require approval of exact arguments. Manage access in Google settings.",
        };
      },
    }),
  };
  for (const definition of available) {
    const write = executableTool(definition.name)!.write;
    tools[definition.name] = tool({
      description: definition.description,
      inputSchema: jsonSchema<Record<string, unknown>>({
        ...definition.inputSchema,
        required: [
          ...new Set([...((definition.inputSchema.required as string[]) ?? []), "connectionId"]),
        ],
      }),
      execute: async (input, options) => {
        await authorize();
        if (options.abortSignal?.aborted) throw new Error("Action cancelled.");
        if (
          write &&
          !(await approvals.consume(options.toolCallId, definition.name, input, session))
        )
          throw new Error(
            "A fresh approval of these exact arguments is required; nothing was executed.",
          );
        // This is the existing MCP execution path, including fresh grant resolution and auditing.
        const response = await handleToolCall(
          options.toolCallId,
          { name: definition.name, arguments: input },
          env,
          repo,
          session,
        );
        const body = response.body as { result?: unknown; error?: unknown };
        return (
          body.result ?? {
            isError: true,
            message: "Google action unavailable or denied. Check Google settings before retrying.",
          }
        );
      },
    });
  }
  return tools;
}
