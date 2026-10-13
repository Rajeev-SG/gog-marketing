import { once } from "node:events";
import { createServer, type Server } from "node:http";
import { describe, expect, it } from "vitest";
import type { NativeIdentityProvider } from "./runner-auth.js";
import { RemoteToolRunner, type ToolExecutionRequest } from "./tool-runner.js";

const INVOCATION_TOKEN = "x".repeat(32);
const NATIVE_ID_TOKEN = "native-identity-token";
const GOOGLE_ACCESS_TOKEN = "google-access-token";

async function listen(server: Server): Promise<string> {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("test server has no port");
  return `http://127.0.0.1:${address.port}`;
}

async function close(server: Server): Promise<void> {
  const closed = once(server, "close");
  server.close();
  server.closeAllConnections();
  await closed;
}

describe("RemoteToolRunner transport", () => {
  it("signs a read-only native invocation with trusted resource and actual scopes", async () => {
    let sent: Record<string, unknown> | undefined;
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async (_url, init) => {
        sent = JSON.parse(new TextDecoder().decode(init!.body as Uint8Array));
        return Response.json({
          operation: "tool_call",
          ok: true,
          result: { rows: [{ metric: 3 }] },
        });
      }) as typeof fetch,
    });
    const result = await runner.execute({
      tenantId: "trusted-tenant",
      connectionId: "trusted-connection",
      service: "analytics",
      resourceType: "property",
      resourceId: "properties/123",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "owner@example.com",
      googleSubject: "trusted-sub",
      nativeTool: {
        name: "analytics_report",
        arguments: { property: "properties/123" },
        scopes: ["https://www.googleapis.com/auth/analytics.readonly"],
        allowedTools: ["analytics_report"],
        allowWrite: false,
        allowDestructive: false,
      },
    });
    expect(sent).toMatchObject({
      operation: "tool_call",
      tenant_id: "trusted-tenant",
      connection_id: "trusted-connection",
      tool: {
        name: "analytics_report",
        arguments: { property: "properties/123" },
        authorization: {
          allowed_tools: ["analytics_report"],
          scopes: ["https://www.googleapis.com/auth/analytics.readonly"],
          resource: { service: "analytics", resource_id: "properties/123", enabled: true },
          allow_write: false,
          allow_destructive: false,
        },
      },
    });
    expect(sent).not.toHaveProperty("resource");
    expect(result).toEqual({ status: "ok", result: { rows: [{ metric: 3 }] } });
    expect(JSON.stringify(result)).not.toContain(GOOGLE_ACCESS_TOKEN);
  });

  it("returns only non-secret response diagnostics for non-2xx responses", async () => {
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async () =>
        new Response("secret body contents", {
          status: 401,
          headers: { "X-Secret-Header": "secret-header-value" },
        })) as typeof fetch,
    });

    const outcome = await runner.execute({
      tenantId: "tenant",
      connectionId: "connection",
      service: "analytics",
      resourceType: "property",
      resourceId: "properties/123",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "user@example.test",
      googleSubject: "google-subject",
    });

    expect(outcome).toEqual({
      status: "error",
      detail: { runnerStatus: 401, runnerBodyJson: false },
    });
    expect(JSON.stringify(outcome)).not.toContain("secret");
    expect(JSON.stringify(outcome)).not.toContain(GOOGLE_ACCESS_TOKEN);
    expect(JSON.stringify(outcome)).not.toContain(NATIVE_ID_TOKEN);
    expect(JSON.stringify(outcome)).not.toContain("runner.example.test");
  });

  it("returns only non-secret response diagnostics for an unparsable body", async () => {
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async () =>
        new Response("not-json", {
          status: 200,
          headers: { "X-Secret-Header": "secret-header-value" },
        })) as typeof fetch,
    });

    const outcome = await runner.execute({
      tenantId: "tenant",
      connectionId: "connection",
      service: "analytics",
      resourceType: "property",
      resourceId: "properties/123",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "user@example.test",
      googleSubject: "google-subject",
    });

    expect(outcome).toEqual({
      status: "error",
      detail: { runnerStatus: 200, runnerBodyJson: false },
    });
    expect(JSON.stringify(outcome)).not.toContain("not-json");
    expect(JSON.stringify(outcome)).not.toContain("secret");
    expect(JSON.stringify(outcome)).not.toContain(GOOGLE_ACCESS_TOKEN);
    expect(JSON.stringify(outcome)).not.toContain(NATIVE_ID_TOKEN);
    expect(JSON.stringify(outcome)).not.toContain("runner.example.test");
  });

  it("exposes only the known workspace_required runner code with no secrets", async () => {
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async () =>
        Response.json(
          {
            operation: "tool_call",
            ok: false,
            error: {
              code: "workspace_required",
              message: "Drive Labels API requires a customer; SECRET_TOKEN user@example.com",
            },
          },
          { status: 422 },
        )) as typeof fetch,
    });

    const outcome = await runner.execute({
      tenantId: "tenant",
      connectionId: "connection",
      service: "drivelabels",
      resourceType: "",
      resourceId: "",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "user@example.com",
      googleSubject: "google-subject",
      nativeTool: {
        name: "drivelabels_list",
        arguments: {},
        scopes: ["https://www.googleapis.com/auth/drive.labels.readonly"],
        allowedTools: ["drivelabels_list"],
        allowWrite: false,
        allowDestructive: false,
      },
    });

    expect(outcome).toEqual({
      status: "error",
      detail: { runnerStatus: 422, runnerBodyJson: true, runnerCode: "workspace_required" },
    });
    const serialized = JSON.stringify(outcome);
    expect(serialized).not.toContain("SECRET_TOKEN");
    expect(serialized).not.toContain("user@example.com");
    expect(serialized).not.toContain(GOOGLE_ACCESS_TOKEN);
    expect(serialized).not.toContain(NATIVE_ID_TOKEN);
  });

  it.each([
    { status: 502, operation: "tool_call", ok: false },
    { status: 422, operation: "discover", ok: false },
    { status: 422, operation: "tool_call", ok: true },
  ])("omits runnerCode for a mismatched error envelope %j", async (envelope) => {
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async () =>
        Response.json(
          {
            operation: envelope.operation,
            ok: envelope.ok,
            error: { code: "workspace_required" },
          },
          { status: envelope.status },
        )) as typeof fetch,
    });
    const outcome = await runner.execute({
      tenantId: "tenant",
      connectionId: "connection",
      service: "drivelabels",
      resourceType: "",
      resourceId: "",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "user@example.com",
      googleSubject: "google-subject",
      nativeTool: {
        name: "drivelabels_list",
        arguments: {},
        scopes: ["https://www.googleapis.com/auth/drive.labels.readonly"],
        allowedTools: ["drivelabels_list"],
        allowWrite: false,
        allowDestructive: false,
      },
    });
    expect(outcome).toEqual({
      status: "error",
      detail: {
        runnerStatus: envelope.status,
        runnerBodyJson: true,
      },
    });
  });

  it("omits runnerCode for unknown error codes", async () => {
    const runner = new RemoteToolRunner({
      url: new URL("https://runner.example.test/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity: {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      },
      fetchImpl: (async () =>
        Response.json(
          { operation: "tool_call", ok: false, error: { code: "mystery" } },
          { status: 422 },
        )) as typeof fetch,
    });

    const outcome = await runner.execute({
      tenantId: "tenant",
      connectionId: "connection",
      service: "drivelabels",
      resourceType: "",
      resourceId: "",
      accessToken: GOOGLE_ACCESS_TOKEN,
      googleEmail: "user@example.com",
      googleSubject: "google-subject",
      nativeTool: {
        name: "drivelabels_list",
        arguments: {},
        scopes: ["https://www.googleapis.com/auth/drive.labels.readonly"],
        allowedTools: ["drivelabels_list"],
        allowWrite: false,
        allowDestructive: false,
      },
    });

    expect(outcome).toEqual({
      status: "error",
      detail: { runnerStatus: 422, runnerBodyJson: true },
    });
  });

  it("rejects redirect responses without forwarding credential headers or body", async () => {
    const redirectedRequests: Array<{ headers: string; body: string }> = [];
    const target = createServer((request, response) => {
      let body = "";
      request.setEncoding("utf8");
      request.on("data", (chunk: string) => {
        body += chunk;
      });
      request.on("end", () => {
        redirectedRequests.push({ headers: JSON.stringify(request.headers), body });
        response.writeHead(200, { "Content-Type": "application/json" });
        response.end("{}");
      });
    });
    const redirector = createServer((_request, response) => {
      response.writeHead(308, { Location: targetUrl });
      response.end();
    });

    let targetUrl = "";
    try {
      targetUrl = `${await listen(target)}/capture`;
      const runnerUrl = new URL(`${await listen(redirector)}/execute`);
      const identity: NativeIdentityProvider = {
        async nativeIdToken() {
          return NATIVE_ID_TOKEN;
        },
      };
      const runner = new RemoteToolRunner({
        url: runnerUrl,
        invocationToken: INVOCATION_TOKEN,
        identity,
      });
      const request: ToolExecutionRequest = {
        tenantId: "tenant",
        connectionId: "connection",
        service: "analytics",
        resourceType: "property",
        resourceId: "properties/123",
        accessToken: GOOGLE_ACCESS_TOKEN,
        googleEmail: "user@example.test",
        googleSubject: "google-subject",
      };

      await expect(runner.execute(request)).resolves.toEqual({
        status: "error",
        detail: "runner_redirect_blocked",
      });
      expect(redirectedRequests).toEqual([]);
    } finally {
      await Promise.all([close(target), close(redirector)]);
    }
  });
});
