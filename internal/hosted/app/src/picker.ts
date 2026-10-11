import {
  googleGrantForConnection,
  ConnectError,
  type ConnectDeps,
  type ConnectSession,
} from "./connect.js";

const DRIVE_FILE = "https://www.googleapis.com/auth/drive.file";
const FILE_ID = /^[A-Za-z0-9_-]{1,256}$/;
export interface PickerConfig {
  apiKey: string;
  appId: string;
}

async function digest(value: string): Promise<string> {
  const bytes = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value));
  return [...new Uint8Array(bytes)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

export async function startPicker(
  deps: ConnectDeps,
  session: ConnectSession,
  connectionId: string,
  config: PickerConfig,
) {
  const connection = await deps.repo.getConnection(session.tenantId, connectionId);
  if (!connection) throw new ConnectError("connection_not_found", 404);
  if (!config.apiKey || !/^[0-9]+$/.test(config.appId))
    throw new ConnectError("operator_config_missing", 503);
  if (!deps.tools?.verifyFiles) throw new ConnectError("discovery_unavailable", 503);
  if (connection.status !== "active" || !connection.googleSubject)
    throw new ConnectError("needs_reconnect", 409);
  const grant = await googleGrantForConnection(deps, session, connection);
  if (!grant.scopes.includes(DRIVE_FILE)) throw new ConnectError("needs_reconnect", 409);
  const token = [...crypto.getRandomValues(new Uint8Array(32))]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
  const now = deps.now?.() ?? new Date();
  const quota = await deps.repo.incrementQuotaWithinLimit(
    session.tenantId,
    now.toISOString().slice(0, 10),
    "picker_sessions",
    50,
  );
  if (quota === null) throw new ConnectError("picker_rate_limited", 429);
  await deps.repo.createPickerSession({
    sessionHash: await digest(token),
    tenantId: session.tenantId,
    connectionId,
    clerkSessionId: session.sessionId,
    googleSubject: connection.googleSubject,
    expiresAt: new Date(now.getTime() + 300_000).toISOString(),
  });
  // Google Picker's official JS API requires an ephemeral browser token.
  // Never return refresh/client-secret material, put this in a URL/storage/log,
  // or use this response for generic Google execution in the browser.
  return {
    pickerSession: token,
    accessToken: grant.accessToken,
    apiKey: config.apiKey,
    appId: config.appId,
    googleEmail: connection.email,
  };
}

const MIME_SERVICE: Record<string, string> = {
  "application/vnd.google-apps.document": "docs",
  "application/vnd.google-apps.spreadsheet": "sheets",
  "application/vnd.google-apps.presentation": "slides",
  "application/vnd.google-apps.site": "sites",
};

export async function completePicker(deps: ConnectDeps, session: ConnectSession, raw: unknown) {
  if (!raw || typeof raw !== "object" || Array.isArray(raw))
    throw new ConnectError("invalid_request", 400);
  const body = raw as Record<string, unknown>;
  if (
    Object.keys(body).some((key) => !["connectionId", "pickerSession", "fileIds"].includes(key)) ||
    typeof body.connectionId !== "string" ||
    typeof body.pickerSession !== "string" ||
    !/^[a-f0-9]{64}$/.test(body.pickerSession) ||
    !Array.isArray(body.fileIds) ||
    body.fileIds.length === 0 ||
    body.fileIds.length > 20
  )
    throw new ConnectError("invalid_request", 400);
  const ids: string[] = [];
  for (const id of body.fileIds) {
    if (typeof id !== "string" || !FILE_ID.test(id) || ids.includes(id))
      throw new ConnectError("invalid_request", 400);
    ids.push(id);
  }
  const connection = await deps.repo.getConnection(session.tenantId, body.connectionId);
  if (!connection) throw new ConnectError("connection_not_found", 404);
  if (connection.status !== "active") throw new ConnectError("needs_reconnect", 409);
  const consumed = await deps.repo.consumePickerSession({
    sessionHash: await digest(body.pickerSession),
    tenantId: session.tenantId,
    connectionId: connection.id,
    clerkSessionId: session.sessionId,
    googleSubject: connection.googleSubject,
    now: (deps.now?.() ?? new Date()).toISOString(),
  });
  if (!consumed) throw new ConnectError("state_unknown", 400);
  const grant = await googleGrantForConnection(deps, session, connection);
  if (!grant.scopes.includes(DRIVE_FILE)) throw new ConnectError("needs_reconnect", 409);
  if (!deps.tools?.verifyFiles) throw new ConnectError("discovery_unavailable", 503);
  const result = await deps.tools.verifyFiles({
    tenantId: session.tenantId,
    connectionId: connection.id,
    googleEmail: connection.email,
    googleSubject: connection.googleSubject,
    accessToken: grant.accessToken,
    scopes: grant.scopes,
    ids,
  });
  if (result.status !== "ok") throw new ConnectError("discovery_error", 502);
  const resources: Array<{
    service: string;
    resourceId: string;
    resourceType: string;
    displayName: string;
    enabled: boolean;
    metadataJson: string;
  }> = [];
  const accounted = [...result.files.map((file) => file.id), ...result.rejected];
  if (
    accounted.length !== ids.length ||
    new Set(accounted).size !== ids.length ||
    accounted.some((id) => !ids.includes(id))
  )
    throw new ConnectError("discovery_error", 502);
  for (const file of result.files) {
    const services = ["drive", MIME_SERVICE[file.mime_type]].filter((value): value is string =>
      Boolean(value),
    );
    for (const service of services)
      resources.push({
        service,
        resourceId: file.id,
        resourceType: "file",
        displayName: file.name,
        enabled: false,
        metadataJson: JSON.stringify({ mime_type: file.mime_type, source: "picker" }),
      });
  }
  // At most 40 rows fit one native D1 transactional batch; existing enablement
  // survives conflict and a failed batch cannot partially expand inventory.
  await deps.repo.upsertResourceGrants(session.tenantId, connection.id, resources);
  await deps.repo.appendAudit({
    id: crypto.randomUUID(),
    tenantId: session.tenantId,
    connectionId: connection.id,
    actorClerkUserId: session.userId,
    action: "google.picker.complete",
    result: "allow",
    detailJson: JSON.stringify({ operation: "select", count: result.files.length, error: "none" }),
    latencyMs: null,
  });
  return { selectedCount: result.files.length, rejectedCount: result.rejected.length };
}
