import { beforeEach, afterEach, describe, it, expect } from "vitest";
import {
  createTestDatabase,
  applyMigrationSql,
  type SQLiteD1Adapter,
} from "../../state/src/sqlite-adapter.js";
import { loadMigrationSql } from "../../state/src/dev-migrations.js";
import { HostedRepository } from "../../state/src/repository.js";
import { CredentialCipher, parseCredentialKeys } from "./credential-crypto.js";
import { GoogleOAuthClient } from "./google-oauth.js";
import { UnavailableDiscoveryRunner } from "./discovery.js";
import type { ConnectDeps, ConnectSession } from "./connect.js";
import { startPicker, completePicker } from "./picker.js";
import type { SelectedFilesRequest } from "./tool-runner.js";

describe("Picker account/session and verified-file grants", () => {
  let db: SQLiteD1Adapter;
  let deps: ConnectDeps;
  let session: ConnectSession;
  let connectionId: string;
  const requests: SelectedFilesRequest[] = [];
  beforeEach(async () => {
    db = createTestDatabase();
    for (const file of [
      "0001_initial_schema.sql",
      "0002_oauth_states_discovery.sql",
      "0006_picker_sessions.sql",
    ])
      applyMigrationSql(db, loadMigrationSql(file));
    const repo = new HostedRepository(db);
    const tenant = await repo.bootstrapTenant("owner");
    const connection = await repo.createConnection(
      tenant.id,
      "google-a",
      "a@example.com",
      "",
      "[]",
    );
    connectionId = connection.id;
    session = { tenantId: tenant.id, userId: "owner", sessionId: "clerk-session-a" };
    const cipher = new CredentialCipher(
      parseCredentialKeys(Buffer.alloc(32, 4).toString("base64")),
    );
    const encrypted = await cipher.encrypt(tenant.id, connectionId, {
      access_token: "ephemeral-browser-token",
      refresh_token: "private-refresh",
      token_type: "Bearer",
      expiry: new Date(Date.now() + 3600000).toISOString(),
      granted_scopes: ["https://www.googleapis.com/auth/drive.file"],
    });
    await repo.upsertCredential({
      tenantId: tenant.id,
      connectionId,
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: Number(encrypted.keyVersion),
    });
    requests.length = 0;
    deps = {
      repo,
      cipher,
      oauth: new GoogleOAuthClient({ clientId: "client", clientSecret: "private-secret" }),
      discovery: new UnavailableDiscoveryRunner(),
      redirectUri: "https://app.example/callback",
      stateTtlSeconds: 600,
      tools: {
        async execute() {
          return { status: "unavailable", detail: "unused" };
        },
        async verifyFiles(request) {
          requests.push(request);
          return {
            status: "ok",
            files: request.ids
              .filter((id) => id !== "denied")
              .map((id) => ({
                id,
                name: "Google verified name",
                mime_type: "application/vnd.google-apps.document",
              })),
            rejected: request.ids.filter((id) => id === "denied"),
          };
        },
      },
    };
  });
  afterEach(() => db.close());
  it("uses authoritative Forms MIME to create disabled form and bound-script parent grants", async () => {
    deps.tools!.verifyFiles = async (request) => ({
      status: "ok",
      files: request.ids.map((id) => ({
        id,
        name: "Google form",
        mime_type: "application/vnd.google-apps.form",
      })),
      rejected: [],
    });
    const config = await startPicker(deps, session, connectionId, {
      apiKey: "public-key",
      appId: "123",
    });
    await completePicker(deps, session, {
      connectionId,
      pickerSession: config.pickerSession,
      fileIds: ["selected-form"],
    });
    const grants = await deps.repo.listResourceGrants(session.tenantId, connectionId);
    expect(
      grants
        .filter((grant) => grant.resourceId === "selected-form")
        .map((grant) => grant.service)
        .sort(),
    ).toEqual(["appscript", "drive", "forms"]);
    expect(grants.every((grant) => !grant.enabled)).toBe(true);
  });

  it("returns only ephemeral Picker material and saves verified files disabled with existing choices preserved", async () => {
    const config = await startPicker(deps, session, connectionId, {
      apiKey: "restricted-public-key",
      appId: "123",
    });
    expect(config.accessToken).toBe("ephemeral-browser-token");
    expect(JSON.stringify(config)).not.toContain("private-refresh");
    expect(JSON.stringify(config)).not.toContain("private-secret");
    await deps.repo.upsertResourceGrant(
      session.tenantId,
      connectionId,
      "docs",
      "old-doc",
      "file",
      "Old",
      true,
      "{}",
    );
    const result = await completePicker(deps, session, {
      connectionId,
      pickerSession: config.pickerSession,
      fileIds: ["new-doc", "old-doc", "denied"],
    });
    expect(result).toEqual({ selectedCount: 2, rejectedCount: 1 });
    const grants = await deps.repo.listResourceGrants(session.tenantId, connectionId);
    expect(grants.find((g) => g.service === "docs" && g.resourceId === "old-doc")!.enabled).toBe(
      true,
    );
    expect(grants.filter((g) => g.resourceId === "new-doc").every((g) => !g.enabled)).toBe(true);
    expect(grants.some((g) => g.resourceId === "denied")).toBe(false);
    expect(requests[0]!.googleSubject).toBe("google-a");
    await expect(
      completePicker(deps, session, {
        connectionId,
        pickerSession: config.pickerSession,
        fileIds: ["new-doc"],
      }),
    ).rejects.toMatchObject({ code: "state_unknown" });
  });
  it("rejects sibling sessions, foreign accounts and spoofed file metadata before verification", async () => {
    const config = await startPicker(deps, session, connectionId, { apiKey: "key", appId: "123" });
    const body = { connectionId, pickerSession: config.pickerSession, fileIds: ["doc"] };
    await expect(
      completePicker(deps, { ...session, sessionId: "clerk-session-b" }, body),
    ).rejects.toMatchObject({ code: "state_unknown" });
    await expect(
      completePicker(deps, session, { ...body, fileIds: ["../secret"] }),
    ).rejects.toMatchObject({ code: "invalid_request" });
    await expect(
      completePicker(deps, session, { ...body, googleEmail: "other@example.com" }),
    ).rejects.toMatchObject({ code: "invalid_request" });
    const other = await deps.repo.createConnection(
      session.tenantId,
      "google-b",
      "b@example.com",
      "",
      "[]",
    );
    await expect(
      completePicker(deps, session, { ...body, connectionId: other.id }),
    ).rejects.toMatchObject({ code: "state_unknown" });
    expect(requests).toEqual([]);
    await completePicker(deps, session, body);
    expect(requests).toHaveLength(1);
  });
  it("caps Picker issuance without returning another ephemeral token", async () => {
    const period = new Date().toISOString().slice(0, 10);
    for (let i = 0; i < 50; i++)
      await deps.repo.incrementQuotaWithinLimit(session.tenantId, period, "picker_sessions", 50);
    await expect(
      startPicker(deps, session, connectionId, { apiKey: "key", appId: "123" }),
    ).rejects.toMatchObject({ code: "picker_rate_limited", status: 429 });
  });
});
