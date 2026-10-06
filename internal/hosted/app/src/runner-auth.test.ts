import { beforeEach, describe, expect, it } from "vitest";
import {
  clearNativeIdentityCacheForTest,
  signRunnerCapabilityJwt,
  WorkloadIdentityClient,
  type RunnerIdentityConfig,
} from "./runner-auth.js";

const PROVIDER =
  "projects/629716276051/locations/global/workloadIdentityPools/test-pool/providers/test-provider";
const ISSUER = "https://identity.example.com/";
const SERVICE_ACCOUNT = "runner@project.iam.gserviceaccount.com";

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

function encode(value: string): string {
  return Buffer.from(value, "utf8").toString("base64url");
}

async function verifyCapability(
  token: string,
  secret: string,
  body: Uint8Array,
): Promise<Record<string, unknown>> {
  const [header, payload, signature] = token.split(".");
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["verify"],
  );
  expect(
    await crypto.subtle.verify(
      "HMAC",
      key,
      Buffer.from(signature!, "base64url"),
      new TextEncoder().encode(`${header}.${payload}`),
    ),
  ).toBe(true);
  const claims = JSON.parse(decode(payload!)) as Record<string, unknown>;
  const digest = await crypto.subtle.digest("SHA-256", body);
  expect(claims.request_sha256).toBe(Buffer.from(digest).toString("hex"));
  return claims;
}

describe("runner authentication", () => {
  beforeEach(() => {
    clearNativeIdentityCacheForTest();
  });

  it("signs the capability over exact UTF-8 body bytes with the Go context", async () => {
    const secret = "x".repeat(32);
    const body = new TextEncoder().encode('{"exact":"json"}');
    const now = new Date("2026-10-06T12:00:00Z");
    const token = await signRunnerCapabilityJwt(
      secret,
      { tenantId: "tenant", connectionId: "connection", operation: "discover" },
      body,
      now,
    );
    const [header] = token.split(".");
    expect(JSON.parse(decode(header!))).toEqual({ alg: "HS256", typ: "JWT" });
    const claims = await verifyCapability(token, secret, body);
    expect(claims).toMatchObject({
      iss: "gog-marketing-worker",
      sub: "gog-marketing-worker",
      aud: "gog-marketing-runner",
      tenant_id: "tenant",
      connection_id: "connection",
      operation: "discover",
    });
    expect(Number(claims.exp) - Number(claims.iat)).toBeLessThanOrEqual(60);
    expect(Number(claims.iat)).toBe(Math.floor(now.getTime() / 1000));
    await expect(verifyCapability(token, secret, new TextEncoder().encode("{}"))).rejects.toThrow();
  });

  it("rejects invocation secrets shorter than the Go contract", async () => {
    await expect(
      signRunnerCapabilityJwt(
        "short-secret",
        { tenantId: "tenant", connectionId: "connection", operation: "discover" },
        new TextEncoder().encode("{}"),
      ),
    ).rejects.toThrow("invocation secret is too short");
  });

  it("exchanges WIF, mints the native ID token, and caches only that identity", async () => {
    const config = await testIdentityConfig();
    const calls: Array<{ url: string; init: RequestInit }> = [];
    let count = 0;
    const identity = new WorkloadIdentityClient({
      config,
      now: () => new Date("2026-10-06T12:00:00Z"),
      fetchImpl: (async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ url: String(input), init: init ?? {} });
        count += 1;
        if (String(input) === "https://sts.googleapis.com/v1/token") {
          return Response.json({ access_token: `federated-access-${count}` });
        }
        const exp = Math.floor((Date.parse("2026-10-06T12:04:00Z") || 0) / 1000);
        return Response.json({ token: `header.${encode(JSON.stringify({ exp }))}.signature` });
      }) as typeof fetch,
    });

    const first = await identity.nativeIdToken("https://runner.example.com", undefined);
    expect(await identity.nativeIdToken("https://runner.example.com", undefined)).toBe(first);
    expect(calls).toHaveLength(2);
    expect(calls[0]!.url).toBe("https://sts.googleapis.com/v1/token");
    const body = JSON.parse(String(calls[0]!.init.body)) as Record<string, string>;
    expect(body).toEqual({
      grantType: "urn:ietf:params:oauth:grant-type:token-exchange",
      audience: `//iam.googleapis.com/${PROVIDER}`,
      scope: "https://www.googleapis.com/auth/cloud-platform",
      requestedTokenType: "urn:ietf:params:oauth:token-type:access_token",
      subjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
      subjectToken: expect.any(String),
    });
    const [callerHeader, callerPayload] = body.subjectToken.split(".");
    expect(JSON.parse(decode(callerHeader!))).toEqual({
      alg: "RS256",
      typ: "JWT",
      kid: "test-kid",
    });
    const iat = Math.floor(Date.parse("2026-10-06T12:00:00Z") / 1000);
    expect(JSON.parse(decode(callerPayload!))).toEqual({
      iss: ISSUER,
      sub: "gog-marketing-worker",
      aud: `https://iam.googleapis.com/${PROVIDER}`,
      iat,
      exp: iat + 300,
    });
    expect(calls[1]!.url).toBe(
      `https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/${encodeURIComponent(
        SERVICE_ACCOUNT,
      )}:generateIdToken`,
    );
    expect(calls[1]!.init.headers).toMatchObject({
      Authorization: "Bearer federated-access-1",
      "Content-Type": "application/json",
    });
    expect(JSON.parse(String(calls[1]!.init.body))).toEqual({
      audience: "https://runner.example.com",
      includeEmail: true,
    });
  });

  it("refreshes the scoped native identity only after expiry", async () => {
    const config = await testIdentityConfig();
    const calls: string[] = [];
    let count = 0;
    let now = Date.parse("2026-10-06T12:00:00Z");
    const identity = new WorkloadIdentityClient({
      config,
      now: () => new Date(now),
      fetchImpl: (async (input: RequestInfo | URL) => {
        calls.push(String(input));
        count += 1;
        if (String(input).endsWith("/token")) {
          return Response.json({ access_token: `access-${count}` });
        }
        const exp = Math.floor((now + 61_000) / 1000);
        return Response.json({ token: `header.${encode(JSON.stringify({ exp }))}.signature` });
      }) as typeof fetch,
    });
    const first = await identity.nativeIdToken("https://runner.example.com", undefined);
    expect(await identity.nativeIdToken("https://runner.example.com", undefined)).toBe(first);
    expect(calls).toHaveLength(2);
    now += 120_000;
    expect(await identity.nativeIdToken("https://runner.example.com", undefined)).not.toBe(first);
    expect(calls).toHaveLength(4);
  });

  it("rejects invalid operator identity configuration without leaking details", () => {
    expect(
      () =>
        new WorkloadIdentityClient({
          config: {
            provider: "projects/x",
            issuer: ISSUER,
            keyId: "kid",
            signingKey: Buffer.from("not-a-key").toString("base64"),
            serviceAccount: SERVICE_ACCOUNT,
          },
        }),
    ).toThrow("identity provider is not configured");
  });
});
