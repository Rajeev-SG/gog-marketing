/**
 * Remote MCP Streamable HTTP for hosted v1 (#65).
 *
 * This is deliberately a small protocol surface: initialize, ping, tools/list,
 * and policy-checked tools/call routing. Tool execution belongs to #66 and is
 * always rejected here. Tenant and resource authority comes only from the
 * verified Clerk identity and tenant-scoped D1 grants.
 *
 * Clerk DCR is proxied only when Clerk's public OAuth metadata advertises a
 * same-origin HTTPS registration endpoint. When it does not, /oauth/register
 * returns `registration_unavailable`; an operator must manually configure a
 * public OAuth client with PKCE S256 (or enable Clerk CIMD/DCR) before clients
 * can register.
 */
import { authenticateBearer, bearerToken, validateIssuer, type ClerkEnv } from "./auth.js";
import { HostedRepository } from "../../state/src/repository.js";
import type { Tenant } from "../../state/src/types.js";

export const MCP_PROTOCOL_VERSIONS = ["2025-11-25", "2025-06-18", "2025-03-26"] as const;
const CURRENT_MCP_PROTOCOL_VERSION = MCP_PROTOCOL_VERSIONS[0];
const JSONRPC_HEADERS = {
  "Content-Type": "application/json; charset=utf-8",
  "Cache-Control": "no-store",
  "X-Content-Type-Options": "nosniff",
} as const;

export interface McpEnv extends ClerkEnv {
  GOG_HOSTED_CANONICAL_ORIGIN?: string;
  __testFetch?: typeof fetch;
}

type JsonRpcId = string | number | null;
interface JsonRpcMessage {
  jsonrpc?: unknown;
  id?: unknown;
  method?: unknown;
  params?: unknown;
  result?: unknown;
  error?: unknown;
}

interface McpTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  annotations?: Record<string, boolean>;
}

function schema(
  properties: Record<string, unknown> = {},
  required: string[] = [],
): Record<string, unknown> {
  return { type: "object", properties, required, additionalProperties: false };
}

const STRING = { type: "string" } as const;
const INTEGER = (minimum: number, maximum: number, defaultValue?: number) => ({
  type: "integer",
  minimum,
  maximum,
  ...(defaultValue === undefined ? {} : { default: defaultValue }),
});

function tool(
  name: string,
  description: string,
  properties: Record<string, unknown> = {},
  required: string[] = [],
): McpTool {
  return {
    name,
    description,
    inputSchema: schema(properties, required),
    annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true },
  };
}

// Names, descriptions, and argument contracts mirror internal/cmd/mcp_marketing.go.
const TOOL_CATALOG: Record<string, McpTool[]> = {
  analytics: [
    tool("analytics_accounts", "List GA4 account summaries.", {
      max: INTEGER(1, 200, 50),
    }),
    tool(
      "analytics_report",
      "Run a GA4 Data API report.",
      {
        property: STRING,
        dimensions: { type: "string", default: "date" },
        metrics: { type: "string", default: "activeUsers" },
        from: { type: "string", default: "7daysAgo" },
        to: { type: "string", default: "today" },
        max: INTEGER(1, 250000, 100),
      },
      ["property"],
    ),
    tool("analytics_properties_list", "List GA4 properties.", { max: INTEGER(1, 200, 50) }),
    tool("analytics_properties_get", "Get one GA4 property.", { property: STRING }, ["property"]),
    tool(
      "analytics_datastreams_list",
      "List GA4 data streams.",
      { property: STRING, max: INTEGER(1, 200, 50) },
      ["property"],
    ),
  ],
  tagmanager: [
    tool("tagmanager_accounts_list", "List GTM accounts."),
    tool("tagmanager_containers_list", "List GTM containers for an account.", { account: STRING }, [
      "account",
    ]),
    tool(
      "tagmanager_workspaces_list",
      "List GTM workspaces.",
      { account: STRING, container: STRING },
      ["account", "container"],
    ),
    ...["tags", "triggers", "variables"].map((kind) =>
      tool(
        `tagmanager_${kind}_list`,
        `List GTM ${kind}.`,
        { account: STRING, container: STRING, workspace: STRING },
        ["account", "container", "workspace"],
      ),
    ),
    tool(
      "tagmanager_versions_list",
      "List GTM container version headers.",
      { account: STRING, container: STRING, workspace: STRING },
      ["account", "container", "workspace"],
    ),
  ],
  googleads: [
    tool("googleads_customers_list", "List accessible Google Ads customers."),
    tool(
      "googleads_query",
      "Run a bounded Google Ads GAQL search.",
      {
        customer_id: STRING,
        query: STRING,
        page_size: INTEGER(1, 10000, 100),
        all: { type: "boolean", default: false },
      },
      ["customer_id", "query"],
    ),
  ],
  searchconsole: [
    tool("searchconsole_sites_list", "List Search Console sites."),
    tool(
      "searchconsole_query",
      "Run a Search Console Search Analytics query.",
      {
        site_url: STRING,
        from: STRING,
        to: STRING,
        dimensions: { type: "string", default: "QUERY" },
        max: INTEGER(1, 25000, 1000),
      },
      ["site_url", "from", "to"],
    ),
    tool("searchconsole_sitemaps_list", "List Search Console sitemaps.", { site_url: STRING }, [
      "site_url",
    ]),
  ],
  bigquery: [
    tool(
      "bigquery_datasets_list",
      "List BigQuery datasets.",
      { project: STRING, max: INTEGER(1, 1000, 100) },
      ["project"],
    ),
    tool(
      "bigquery_tables_list",
      "List BigQuery tables.",
      { project: STRING, dataset: STRING, max: INTEGER(1, 1000, 100) },
      ["project", "dataset"],
    ),
    tool(
      "bigquery_table_get",
      "Get BigQuery table metadata.",
      { project: STRING, dataset: STRING, table: STRING },
      ["project", "dataset", "table"],
    ),
    tool(
      "bigquery_table_schema",
      "Get BigQuery table schema.",
      { project: STRING, dataset: STRING, table: STRING },
      ["project", "dataset", "table"],
    ),
    tool(
      "bigquery_table_rows",
      "Read a bounded number of BigQuery table rows.",
      {
        project: STRING,
        dataset: STRING,
        table: STRING,
        max: INTEGER(1, 10000, 100),
      },
      ["project", "dataset", "table"],
    ),
  ],
};

function jsonRpcResult(id: JsonRpcId, result: unknown) {
  return { jsonrpc: "2.0", id, result };
}

function jsonRpcError(
  id: JsonRpcId,
  code: number,
  message: string,
  data?: Record<string, unknown>,
) {
  return {
    jsonrpc: "2.0",
    id,
    error: { code, message, ...(data ? { data } : {}) },
  };
}

export function jsonResponse(body: unknown, status = 200, headers: HeadersInit = {}): Response {
  return Response.json(body, {
    status,
    headers: { ...JSONRPC_HEADERS, ...Object.fromEntries(new Headers(headers)) },
  });
}

function sseResponse(body: unknown): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      controller.enqueue(encoder.encode("id: 1\nretry: 1000\n\n"));
      controller.enqueue(
        encoder.encode(`id: 2\nevent: message\ndata: ${JSON.stringify(body)}\n\n`),
      );
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}

export function canonicalMcpUrl(env: McpEnv): string | null {
  try {
    const url = new URL((env.GOG_HOSTED_CANONICAL_ORIGIN ?? "").trim());
    if (url.protocol !== "https:" || (url.pathname !== "/" && url.pathname !== "")) return null;
    return `${url.origin}/mcp`;
  } catch {
    return null;
  }
}

function oauthError(error: string, description: string, status = 400): Response {
  return jsonResponse({ error, error_description: description }, status);
}

function allowMetadataCors(response: Response): Response {
  const headers = new Headers(response.headers);
  headers.set("Access-Control-Allow-Origin", "*");
  headers.set("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
  headers.set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version");
  headers.set("Access-Control-Expose-Headers", "WWW-Authenticate");
  return new Response(response.body, { status: response.status, headers });
}

async function clerkMetadata(
  env: McpEnv,
): Promise<{ ok: true; value: Record<string, unknown> } | { ok: false; response: Response }> {
  const issuer = validateIssuer(env.CLERK_ISSUER ?? "", env.CLERK_PUBLISHABLE_KEY ?? "");
  const resource = canonicalMcpUrl(env);
  if (!issuer || !resource) {
    return {
      ok: false,
      response: oauthError("server_error", "OAuth metadata is not configured.", 503),
    };
  }
  try {
    const fetchImpl = env.__testFetch ?? fetch.bind(globalThis);
    const upstream = await fetchImpl(`${issuer}/.well-known/openid-configuration`, {
      headers: { Accept: "application/json" },
    });
    if (!upstream.ok) throw new Error("metadata unavailable");
    const value = (await upstream.json()) as Record<string, unknown>;
    if (value.issuer !== issuer) throw new Error("issuer mismatch");
    return { ok: true, value };
  } catch {
    return {
      ok: false,
      response: oauthError("server_error", "Authorization-server metadata is unavailable.", 502),
    };
  }
}

export async function handleMcpMetadata(
  request: Request,
  env: McpEnv,
  path: string,
): Promise<Response | null> {
  const publicPaths = [
    "/.well-known/oauth-protected-resource",
    "/.well-known/oauth-authorization-server",
    "/oauth/register",
  ];
  if (!publicPaths.includes(path)) return null;
  if (request.method === "OPTIONS") return allowMetadataCors(new Response(null, { status: 204 }));

  if (path === "/.well-known/oauth-protected-resource") {
    if (request.method !== "GET")
      return allowMetadataCors(jsonResponse({ error: "method_not_allowed" }, 405));
    const resource = canonicalMcpUrl(env);
    const issuer = validateIssuer(env.CLERK_ISSUER ?? "", env.CLERK_PUBLISHABLE_KEY ?? "");
    if (!resource || !issuer) {
      return allowMetadataCors(
        oauthError("server_error", "Protected-resource metadata is not configured.", 503),
      );
    }
    return allowMetadataCors(
      jsonResponse({
        resource,
        resource_name: "gog-marketing",
        authorization_servers: [issuer],
        bearer_methods_supported: ["header"],
        scopes_supported: ["openid", "profile", "email", "offline_access"],
      }),
    );
  }

  if (path === "/.well-known/oauth-authorization-server") {
    if (request.method !== "GET")
      return allowMetadataCors(jsonResponse({ error: "method_not_allowed" }, 405));
    const metadata = await clerkMetadata(env);
    if (!metadata.ok) return allowMetadataCors(metadata.response);
    const resource = canonicalMcpUrl(env)!;
    return allowMetadataCors(
      jsonResponse({
        ...metadata.value,
        registration_endpoint: `${new URL(resource).origin}/oauth/register`,
      }),
    );
  }

  if (request.method !== "POST")
    return allowMetadataCors(oauthError("invalid_request", "Registration requires POST.", 405));
  if (request.headers.get("content-type")?.split(";")[0].trim() !== "application/json") {
    return allowMetadataCors(
      oauthError("invalid_client_metadata", "A JSON registration document is required."),
    );
  }
  let registration: unknown;
  try {
    registration = await request.json();
    if (!registration || typeof registration !== "object" || Array.isArray(registration))
      throw new Error();
  } catch {
    return allowMetadataCors(
      oauthError("invalid_client_metadata", "The client registration document is invalid."),
    );
  }
  const metadata = await clerkMetadata(env);
  if (!metadata.ok) return allowMetadataCors(metadata.response);
  const registrationRaw =
    typeof metadata.value.registration_endpoint === "string"
      ? metadata.value.registration_endpoint
      : "";
  let registrationUrl: URL | null = null;
  try {
    registrationUrl = new URL(registrationRaw);
  } catch {
    registrationUrl = null;
  }
  const issuer = validateIssuer(env.CLERK_ISSUER ?? "", env.CLERK_PUBLISHABLE_KEY ?? "")!;
  if (
    !registrationUrl ||
    registrationUrl.protocol !== "https:" ||
    registrationUrl.origin !== new URL(issuer).origin
  ) {
    return allowMetadataCors(
      oauthError(
        "registration_unavailable",
        "Clerk dynamic client registration is not enabled. Manually register a public OAuth client with PKCE S256, or enable Clerk CIMD/DCR.",
        501,
      ),
    );
  }
  const fetchImpl = env.__testFetch ?? fetch.bind(globalThis);
  try {
    const upstream = await fetchImpl(registrationUrl, {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: JSON.stringify(registration),
    });
    const text = await upstream.text();
    return allowMetadataCors(
      new Response(text, {
        status: upstream.status,
        headers: {
          ...JSONRPC_HEADERS,
          "Content-Type": upstream.headers.get("content-type") ?? "application/json",
        },
      }),
    );
  } catch {
    return allowMetadataCors(
      oauthError("server_error", "Client registration is unavailable.", 502),
    );
  }
}

async function permittedServices(repo: HostedRepository, tenantId: string): Promise<Set<string>> {
  const services = new Set<string>();
  const connections = await repo.listConnections(tenantId);
  for (const connection of connections) {
    if (connection.status !== "active") continue;
    for (const grant of await repo.listResourceGrants(tenantId, connection.id)) {
      if (grant.enabled) services.add(grant.service);
    }
  }
  return services;
}

export async function policyTools(repo: HostedRepository, tenant: Tenant): Promise<McpTool[]> {
  if (tenant.status !== "active") return [];
  const services = await permittedServices(repo, tenant.id);
  return Object.entries(TOOL_CATALOG)
    .filter(([service]) => services.has(service))
    .flatMap(([, tools]) => tools);
}

function authChallenge(
  resource: string,
  error = "invalid_token",
  reason: string = error,
): Response {
  const resourceMetadata = `${new URL(resource).origin}/.well-known/oauth-protected-resource`;
  return jsonResponse(
    jsonRpcError(null, -32001, "Bearer authentication is required.", { reason }),
    401,
    {
      "WWW-Authenticate": `Bearer resource_metadata="${resourceMetadata}", error="${error}"`,
    },
  );
}

function originRejected(request: Request, resource: string): boolean {
  const origin = request.headers.get("origin");
  if (!origin) return false;
  return origin !== new URL(resource).origin && origin !== new URL(request.url).origin;
}

async function handleRpc(
  message: JsonRpcMessage,
  repo: HostedRepository,
  tenant: Tenant,
): Promise<{ body: unknown; status: number } | { accepted: true }> {
  const hasId = Object.hasOwn(message, "id");
  const id = hasId ? (message.id as JsonRpcId) : null;
  if (message.jsonrpc !== "2.0") {
    return { body: jsonRpcError(null, -32600, "Invalid JSON-RPC request."), status: 400 };
  }
  if (!hasId && typeof message.method !== "string") return { accepted: true };
  if (
    typeof message.method !== "string" ||
    (hasId && typeof message.id !== "string" && typeof message.id !== "number")
  ) {
    return { body: jsonRpcError(null, -32600, "Invalid JSON-RPC request."), status: 400 };
  }
  if (!hasId) return { accepted: true };

  const method = message.method;
  const params = (message.params ?? {}) as Record<string, unknown>;
  if (method === "initialize") {
    const requested = typeof params.protocolVersion === "string" ? params.protocolVersion : "";
    const protocolVersion = (MCP_PROTOCOL_VERSIONS as readonly string[]).includes(requested)
      ? requested
      : CURRENT_MCP_PROTOCOL_VERSION;
    return {
      body: jsonRpcResult(id, {
        protocolVersion,
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: "gog-marketing", title: "gog-marketing", version: "1.0.0" },
      }),
      status: 200,
    };
  }
  if (method === "ping") return { body: jsonRpcResult(id, {}), status: 200 };
  if (method === "tools/list") {
    return { body: jsonRpcResult(id, { tools: await policyTools(repo, tenant) }), status: 200 };
  }
  if (method === "tools/call") {
    const name = typeof params.name === "string" ? params.name : "";
    const tools = await policyTools(repo, tenant);
    if (!name || !tools.some((candidate) => candidate.name === name)) {
      return {
        body: jsonRpcError(id, -32602, "Tool is not permitted for this tenant.", {
          reason: "policy_denied",
        }),
        status: 200,
      };
    }
    return {
      body: jsonRpcError(id, -32001, "Tool execution is not yet implemented.", {
        reason: "not_yet_implemented",
        issue: 66,
      }),
      status: 200,
    };
  }
  return { body: jsonRpcError(id, -32601, "Method not found."), status: 200 };
}

export async function handleMcp(
  request: Request,
  env: McpEnv,
  repo: HostedRepository,
  path: string,
): Promise<Response | null> {
  if (path !== "/mcp") return null;
  const resource = canonicalMcpUrl(env);
  if (!resource)
    return jsonResponse(jsonRpcError(null, -32603, "MCP endpoint is not configured."), 503);
  if (originRejected(request, resource)) {
    return jsonResponse(jsonRpcError(null, -32600, "Invalid request origin."), 403);
  }
  if (request.method === "GET" || request.method === "HEAD" || request.method === "DELETE") {
    return jsonResponse(jsonRpcError(null, -32600, "Streamable GET/SSE is not enabled."), 405, {
      Allow: "POST, OPTIONS",
    });
  }
  if (request.method === "OPTIONS")
    return new Response(null, { status: 204, headers: { Allow: "POST, OPTIONS" } });
  if (request.method !== "POST") {
    return jsonResponse(jsonRpcError(null, -32600, "Unsupported HTTP method."), 405, {
      Allow: "POST, OPTIONS",
    });
  }
  if (!bearerToken(request)) return authChallenge(resource, "invalid_request");
  const auth = await authenticateBearer(request, env, repo, resource);
  if (auth.kind === "unauthenticated") {
    return authChallenge(resource, "invalid_token", auth.reason ?? "invalid_token");
  }
  if (auth.kind === "error")
    return jsonResponse(jsonRpcError(null, -32603, auth.message), auth.status);
  if (auth.session.tenant.status !== "active") {
    return jsonResponse(jsonRpcError(null, -32003, "Tenant access is not available."), 403);
  }
  if (request.headers.get("content-type")?.split(";")[0].trim() !== "application/json") {
    return jsonResponse(jsonRpcError(null, -32600, "A JSON-RPC message is required."), 400);
  }

  let message: unknown;
  try {
    message = await request.json();
  } catch {
    return jsonResponse(jsonRpcError(null, -32700, "Parse error."), 400);
  }
  if (Array.isArray(message) || !message || typeof message !== "object") {
    return jsonResponse(jsonRpcError(null, -32600, "Invalid JSON-RPC request."), 400);
  }
  const rpcMessage = message as JsonRpcMessage;
  const hasId = Object.hasOwn(rpcMessage, "id");
  const id = hasId ? (rpcMessage.id as JsonRpcId) : null;
  if (
    rpcMessage.jsonrpc !== "2.0" ||
    typeof rpcMessage.method !== "string" ||
    (hasId && typeof rpcMessage.id !== "string" && typeof rpcMessage.id !== "number")
  ) {
    return jsonResponse(jsonRpcError(id, -32600, "Invalid JSON-RPC request."), 400);
  }

  let outcome: Awaited<ReturnType<typeof handleRpc>>;
  try {
    outcome = await handleRpc(rpcMessage, repo, auth.session.tenant);
  } catch {
    return jsonResponse(jsonRpcError(id, -32603, "Internal error."), 500);
  }
  if ("accepted" in outcome) return new Response(null, { status: 202 });
  return outcome.status === 200
    ? sseResponse(outcome.body)
    : jsonResponse(outcome.body, outcome.status);
}
