import { describe, expect, it } from "vitest";
import { GoogleOAuthClient, GoogleOAuthError, type GoogleTokenSet } from "./google-oauth.js";

const CLIENT_ID = "629716276051-cq3jnl899hj4ie3f3vhokke8aebc4ff8.apps.googleusercontent.com";

function base64UrlJson(value: unknown): string {
  return Buffer.from(JSON.stringify(value)).toString("base64url");
}

function fakeIdToken(claims: Record<string, unknown>): string {
  return `${base64UrlJson({ alg: "RS256", kid: "test" })}.${base64UrlJson(claims)}.sig`;
}

interface FetchStub {
  fetch: typeof fetch;
  calls: Array<{ url: string; init?: RequestInit }>;
}

function stubFetch(handlers: {
  token?: object | Error;
  userinfo?: object | Error;
  revokeStatus?: number;
  tokenStatus?: number;
}): FetchStub {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const impl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, init });
    if (url.includes("/token")) {
      if (handlers.token instanceof Error) throw handlers.token;
      return new Response(JSON.stringify(handlers.token ?? {}), {
        status: handlers.tokenStatus ?? 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.includes("userinfo")) {
      if (handlers.userinfo instanceof Error) throw handlers.userinfo;
      return new Response(JSON.stringify(handlers.userinfo ?? {}), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.includes("/revoke")) {
      return new Response(null, { status: handlers.revokeStatus ?? 200 });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }) as typeof fetch;
  return { fetch: impl, calls };
}

function client(stub: FetchStub): GoogleOAuthClient {
  return new GoogleOAuthClient({
    clientId: CLIENT_ID,
    clientSecret: "test-secret",
    fetchImpl: stub.fetch,
  });
}

const TOKEN: GoogleTokenSet = {
  accessToken: "ya29.test",
  refreshToken: "1//test",
  tokenType: "Bearer",
  expiry: "2026-10-05T01:00:00.000Z",
  grantedScopes: ["openid", "email"],
  idToken: "",
};

describe("Google OAuth client (#62)", () => {
  it("builds the authorization URL with PKCE S256, offline access, consent, and OIDC nonce", async () => {
    const oauth = client(stubFetch({}));
    const url = new URL(
      await oauth.authorizationURL({
        state: "state-token",
        codeVerifier: "a".repeat(64),
        nonce: "nonce-token",
        scopes: ["openid", "email", "https://www.googleapis.com/auth/analytics.readonly"],
        redirectUri: "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback",
      }),
    );
    expect(url.origin + url.pathname).toBe("https://accounts.google.com/o/oauth2/v2/auth");
    expect(url.searchParams.get("response_type")).toBe("code");
    expect(url.searchParams.get("access_type")).toBe("offline");
    expect(url.searchParams.get("prompt")).toBe("consent");
    expect(url.searchParams.get("include_granted_scopes")).toBe("true");
    expect(url.searchParams.get("state")).toBe("state-token");
    expect(url.searchParams.get("nonce")).toBe("nonce-token");
    expect(url.searchParams.get("code_challenge_method")).toBe("S256");
    const expectedChallenge = Buffer.from(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode("a".repeat(64))),
    ).toString("base64url");
    expect(url.searchParams.get("code_challenge")).toBe(expectedChallenge);
    expect(url.searchParams.get("redirect_uri")).toBe(
      "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback",
    );
  });

  it("exchanges codes with the PKCE verifier and maps success into a token set", async () => {
    const stub = stubFetch({
      token: {
        access_token: "ya29.ok",
        refresh_token: "1//ok",
        expires_in: 3600,
        scope: "openid email",
        token_type: "Bearer",
        id_token: fakeIdToken({ nonce: "nonce-token", aud: CLIENT_ID }),
      },
    });
    const token = await client(stub).exchangeCode({
      code: "auth-code",
      codeVerifier: "verifier",
      nonce: "nonce-token",
      redirectUri: "https://gog-marketing.rajeev-sgill.workers.dev/oauth/google/callback",
    });
    expect(token.accessToken).toBe("ya29.ok");
    expect(token.refreshToken).toBe("1//ok");
    const body = new URLSearchParams(String(stub.calls[0]?.init?.body));
    expect(body.get("grant_type")).toBe("authorization_code");
    expect(body.get("code")).toBe("auth-code");
    expect(body.get("code_verifier")).toBe("verifier");
    expect(body.get("client_secret")).toBe("test-secret");
  });

  it("rejects a nonce/audience mismatch on the exchange", async () => {
    const oauth = client(
      stubFetch({
        token: { access_token: "a", id_token: fakeIdToken({ nonce: "other", aud: CLIENT_ID }) },
      }),
    );
    await expect(
      oauth.exchangeCode({
        code: "c",
        codeVerifier: "v",
        nonce: "expected",
        redirectUri: "https://x/callback",
      }),
    ).rejects.toMatchObject({ kind: "nonce_mismatch" } satisfies Partial<GoogleOAuthError>);
  });

  it("classifies token endpoint failures into distinct kinds", async () => {
    const denied = client(stubFetch({ tokenStatus: 400, token: { error: "access_denied" } }));
    await expect(
      denied.exchangeCode({ code: "c", codeVerifier: "v", nonce: "n", redirectUri: "https://x/c" }),
    ).rejects.toMatchObject({ kind: "consent_denied" });

    const expired = client(stubFetch({ tokenStatus: 400, token: { error: "invalid_grant" } }));
    await expect(
      expired.exchangeCode({
        code: "c",
        codeVerifier: "v",
        nonce: "n",
        redirectUri: "https://x/c",
      }),
    ).rejects.toMatchObject({ kind: "invalid_grant" });

    const down = client(stubFetch({ tokenStatus: 503, token: {} }));
    await expect(
      down.exchangeCode({ code: "c", codeVerifier: "v", nonce: "n", redirectUri: "https://x/c" }),
    ).rejects.toMatchObject({ kind: "api_error" });

    const unreachable = client(stubFetch({ token: new Error("boom") }));
    await expect(
      unreachable.exchangeCode({
        code: "c",
        codeVerifier: "v",
        nonce: "n",
        redirectUri: "https://x/c",
      }),
    ).rejects.toMatchObject({ kind: "network" });
  });

  it("verifies identity with a subject and a verified email", async () => {
    const ok = client(
      stubFetch({
        userinfo: { sub: "sub-1", email: "a@b.test", email_verified: true, name: "A B" },
      }),
    );
    const identity = await ok.identity("token");
    expect(identity).toEqual({
      subject: "sub-1",
      email: "a@b.test",
      emailVerified: true,
      displayName: "A B",
    });

    const missingSub = client(stubFetch({ userinfo: { email: "a@b.test" } }));
    await expect(missingSub.identity("token")).rejects.toMatchObject({ kind: "identity_failed" });

    const unverified = client(
      stubFetch({ userinfo: { sub: "s", email: "a@b.test", email_verified: false } }),
    );
    await expect(unverified.identity("token")).rejects.toMatchObject({ kind: "identity_failed" });
  });

  it("treats HTTP 400 revoke as already-revoked success", async () => {
    const stub = stubFetch({ revokeStatus: 400 });
    await client(stub).revoke("1//token");
    expect(stub.calls.some((c) => c.url.includes("/revoke"))).toBe(true);
  });

  it("keeps the previous refresh token when Google omits it on refresh", async () => {
    const stub = stubFetch({
      token: { access_token: "ya29.new", expires_in: 3600, scope: "openid email" },
    });
    const refreshed = await client(stub).refreshTokenSet({
      refreshToken: "1//keep",
      previous: TOKEN,
    });
    expect(refreshed.refreshToken).toBe("1//keep");
    expect(refreshed.accessToken).toBe("ya29.new");
    const body = new URLSearchParams(String(stub.calls[0]?.init?.body));
    expect(body.get("grant_type")).toBe("refresh_token");
    expect(body.get("refresh_token")).toBe("1//keep");
  });
});
