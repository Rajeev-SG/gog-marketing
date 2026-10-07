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
        detail: "runner_unreachable",
      });
      expect(redirectedRequests).toEqual([]);
    } finally {
      await Promise.all([close(target), close(redirector)]);
    }
  });
});
