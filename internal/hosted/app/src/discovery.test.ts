import { beforeEach, describe, expect, it } from "vitest";
import { RemoteDiscoveryRunner, mapWireReport } from "./discovery.js";
import {
  signRunnerCapabilityJwt,
  WorkloadIdentityClient,
  type RunnerIdentityConfig,
} from "./runner-auth.js";

const PROVIDER =
  "projects/629716276051/locations/global/workloadIdentityPools/test-pool/providers/test-provider";
const ISSUER = "https://identity.example.com/";
const SERVICE_ACCOUNT = "runner@project.iam.gserviceaccount.com";
const INVOCATION_TOKEN = "invocation-secret-at-least-32-bytes";

async function testIdentityConfig(): Promise<RunnerIdentityConfig> {
  const pair = (await crypto.subtle.generateKey(
    {
      name: "RSASSA-PKCS1-v1_5",
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256",
    },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const pkcs8 = await crypto.subtle.exportKey("pkcs8", pair.privateKey);
  return {
    provider: PROVIDER,
    issuer: ISSUER,
    keyId: "test-kid",
    signingKey: Buffer.from(pkcs8).toString("base64"),
    serviceAccount: SERVICE_ACCOUNT,
  };
}

function decode(segment: string): string {
  return Buffer.from(segment.replaceAll("-", "+").replaceAll("_", "/"), "base64").toString();
}

async function runnerWithCalls(
  config: RunnerIdentityConfig,
  runnerResponse: Response | Error = Response.json({
    operation: "discover",
    ok: true,
    result: {
      resources: [],
      statuses: {
        analytics: { state: "ok", resource_count: 0, checked_at: "2026-10-06T00:00:00Z" },
      },
    },
  }),
) {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init });
    const url = String(input);
    if (url === "https://sts.googleapis.com/v1/token") {
      return Response.json({ access_token: "federated-access" });
    }
    if (url.includes(":generateIdToken")) {
      const exp = Math.floor((Date.now() + 300_000) / 1000);
      const payload = Buffer.from(JSON.stringify({ exp })).toString("base64url");
      return Response.json({ token: `header.${payload}.signature` });
    }
    if (runnerResponse instanceof Error) throw runnerResponse;
    return runnerResponse;
  }) as typeof fetch;
  const identity = new WorkloadIdentityClient({ config, fetchImpl });
  const runner = new RemoteDiscoveryRunner({
    url: new URL("https://runner.example.com/v1/execute"),
    invocationToken: INVOCATION_TOKEN,
    identity,
    fetchImpl,
  });
  return { calls, runner };
}

function request() {
  return {
    tenantId: "tenant-1",
    connectionId: "connection-1",
    services: ["analytics"],
    accessToken: "ya29.google-access-token",
    googleEmail: "verified@example.com",
    googleSubject: "google-sub-1",
  };
}

describe("remote discovery", () => {
  beforeEach(async () => {
    // Cache isolation is deliberately simple and test-local.
    await import("./runner-auth.js").then(({ clearNativeIdentityCacheForTest }) =>
      clearNativeIdentityCacheForTest(),
    );
  });

  it("sends the bound capability and native identity only on the private Go request", async () => {
    const config = await testIdentityConfig();
    const { calls, runner } = await runnerWithCalls(
      config,
      Response.json({
        operation: "discover",
        ok: true,
        result: {
          resources: [],
          statuses: {
            analytics: { state: "ok", resource_count: 0, checked_at: "2026-10-06T00:00:00Z" },
          },
        },
      }),
    );
    const outcome = await runner.discover(request());
    expect(outcome.status).toBe("empty");
    expect(calls.map(({ url }) => url)).toEqual([
      "https://sts.googleapis.com/v1/token",
      expect.stringContaining("iamcredentials.googleapis.com"),
      "https://runner.example.com/v1/execute",
    ]);
    const headers = new Headers(calls[2]!.init?.headers);
    const body = new TextDecoder().decode(calls[2]!.init?.body as Uint8Array);
    expect(headers.get("content-type")).toBe("application/json");
    expect(headers.get("authorization")).toMatch(
      /^Bearer [A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/,
    );
    expect(headers.get("x-serverless-authorization")).toMatch(/^Bearer header\..+\.signature$/);
    const parsed = JSON.parse(body) as Record<string, unknown>;
    expect(parsed).toEqual({
      tenant_id: "tenant-1",
      connection_id: "connection-1",
      operation: "discover",
      google_email: "verified@example.com",
      google_subject: "google-sub-1",
      access_token: "ya29.google-access-token",
      services: ["analytics"],
    });
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(body));
    const [, payload] = headers.get("authorization")!.slice(7).split(".");
    const claims = JSON.parse(decode(payload!)) as Record<string, string | number>;
    expect(claims.request_sha256).toBe(Buffer.from(digest).toString("hex"));
    expect(claims).toMatchObject({
      iss: "gog-marketing-worker",
      sub: "gog-marketing-worker",
      aud: "gog-marketing-runner",
      tenant_id: "tenant-1",
      connection_id: "connection-1",
      operation: "discover",
    });
    const signed = await signRunnerCapabilityJwt(
      INVOCATION_TOKEN,
      { tenantId: "tenant-1", connectionId: "connection-1", operation: "discover" },
      new TextEncoder().encode(body),
    );
    const otherPayload = JSON.parse(decode(signed.split(".")[1]!)) as { request_sha256: string };
    expect(otherPayload.request_sha256).toBe(claims.request_sha256);
    expect(body).not.toContain("federated-access");
    expect(JSON.stringify(outcome)).not.toContain("ya29.");
  });

  it("maps all four runner outcomes without inventing resources", () => {
    expect(
      mapWireReport({
        resources: [
          {
            service: "analytics",
            resource_type: "property",
            resource_id: "properties/123",
            display_name: "Test property",
            parent: "accounts/456",
            enabled: false,
          },
        ],
        statuses: {
          analytics: { state: "ok", resource_count: 1, checked_at: "2026-10-06T00:00:00Z" },
        },
      }),
    ).toMatchObject({
      status: "ok",
      resources: [
        {
          service: "analytics",
          resourceType: "property",
          resourceId: "properties/123",
          displayName: "Test property",
          parent: "accounts/456",
          metadata: {},
        },
      ],
      statuses: {
        analytics: { state: "ok", resourceCount: 1, checkedAt: "2026-10-06T00:00:00Z" },
      },
    });
    expect(mapWireReport({ resources: [] })).toEqual({
      status: "empty",
      detail: "no_resources",
      resources: [],
      statuses: {},
    });
    expect(
      mapWireReport({ resources: [], statuses: { analytics: { state: "unavailable" } } }),
    ).toMatchObject({ status: "unavailable" });
    expect(
      mapWireReport({ resources: [], statuses: { analytics: { state: "error" } } }),
    ).toMatchObject({ status: "error" });
  });

  it("uses honest unavailable outcomes for identity failures and preserves redaction", async () => {
    const config = await testIdentityConfig();
    const calls: string[] = [];
    const identity = new WorkloadIdentityClient({
      config,
      fetchImpl: (async (input: RequestInfo | URL) => {
        calls.push(String(input));
        return new Response("x-goog-provider: raw sensitive header", { status: 500 });
      }) as typeof fetch,
    });
    const runner = new RemoteDiscoveryRunner({
      url: new URL("https://runner.example.com/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity,
    });
    const outcome = await runner.discover(request());
    expect(outcome).toEqual({
      status: "unavailable",
      detail: "operator_identity_unavailable",
      resources: [],
      statuses: {},
    });
    expect(calls).toEqual(["https://sts.googleapis.com/v1/token"]);
  });

  it("does not expose raw runner failures or the Google access token", async () => {
    const config = await testIdentityConfig();
    const { runner } = await runnerWithCalls(
      config,
      new Error("raw google exception with ya29.google-access-token"),
    );
    const outcome = await runner.discover(request());
    expect(outcome).toEqual({
      status: "error",
      detail: "runner_unreachable",
      resources: [],
      statuses: {},
    });
    expect(JSON.stringify(outcome)).not.toContain("raw google");
    expect(JSON.stringify(outcome)).not.toContain("ya29.");
  });

  it("does not call native identity or Go for zero-service discovery", async () => {
    const config = await testIdentityConfig();
    const calls: string[] = [];
    const identity = new WorkloadIdentityClient({
      config,
      fetchImpl: (async (input: RequestInfo | URL) => {
        calls.push(String(input));
        throw new Error("unexpected identity call");
      }) as typeof fetch,
    });
    const runner = new RemoteDiscoveryRunner({
      url: new URL("https://runner.example.com/v1/execute"),
      invocationToken: INVOCATION_TOKEN,
      identity,
      fetchImpl: (async (input: RequestInfo | URL) => {
        calls.push(String(input));
        throw new Error("unexpected runner call");
      }) as typeof fetch,
    });
    const outcome = await runner.discover({ ...request(), services: [] });
    expect(outcome).toEqual({
      status: "empty",
      detail: "no_discovery_services",
      resources: [],
      statuses: {},
    });
    expect(calls).toEqual([]);
  });

  it("rejects malformed and foreign-operation success envelopes", async () => {
    const config = await testIdentityConfig();
    for (const wire of [
      { resources: [] },
      { operation: "discover", ok: true },
      {
        operation: "analytics_property_read",
        ok: true,
        result: { resources: [], statuses: {} },
      },
      {
        operation: "discover",
        ok: true,
        result: { resources: [{ service: "analytics" }], statuses: {} },
      },
      {
        operation: "discover",
        ok: true,
        result: {
          resources: [],
          statuses: {
            analytics: { state: "ok", resource_count: 0, checked_at: "2026-10-06T00:00:00Z" },
            tagmanager: { state: "unsupported" },
          },
        },
      },
    ]) {
      const { runner } = await runnerWithCalls(config, Response.json(wire));
      const outcome = await runner.discover(request());
      expect(outcome).toEqual({
        status: "error",
        detail: "runner_invalid_response",
        resources: [],
        statuses: {},
      });
    }
  });
});
