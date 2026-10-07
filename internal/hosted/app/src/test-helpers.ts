/**
 * Test helpers for Clerk-backed auth tests.
 *
 * Generates real RSA key pairs, signs real Clerk-format session JWTs with
 * Clerk's own `signJwt` utility, and provides the public JWK as `jwtKey` so
 * that `authenticateRequest` performs real signature verification (networkless
 * via `jwtKey`, not a custom JWT decoder).
 */
import { signJwt } from "@clerk/backend/jwt";

export interface TestKeyPair {
  privateJwk: JsonWebKey;
  /** PEM public key (SPKI) usable as the Clerk `jwtKey`. */
  publicPem: string;
}

export async function generateTestKey(): Promise<TestKeyPair> {
  const keyPair = (await crypto.subtle.generateKey(
    {
      name: "RSASSA-PKCS1-v1_5",
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256",
    },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const privateJwk = (await crypto.subtle.exportKey("jwk", keyPair.privateKey)) as JsonWebKey;
  const spki = await crypto.subtle.exportKey("spki", keyPair.publicKey);
  const b64 = Buffer.from(spki).toString("base64");
  const lines = b64.match(/.{1,64}/g)?.join("\n") ?? "";
  const publicPem = `-----BEGIN PUBLIC KEY-----\n${lines}\n-----END PUBLIC KEY-----`;
  return { privateJwk, publicPem };
}

export interface TestTokenPayload {
  sub: string;
  azp: string;
  typ?: string;
  exp?: number;
  iat?: number;
  nbf?: number;
  sid?: string;
  iss?: string;
  aud?: string;
}

export async function signTestToken(
  payload: TestTokenPayload,
  privateJwk: JsonWebKey,
): Promise<string> {
  const exp = payload.exp ?? Math.floor(Date.now() / 1000) + 300;
  const iat = payload.iat ?? Math.floor(Date.now() / 1000);
  const nbf = payload.nbf ?? iat;
  const sid = payload.sid ?? "sess_test_123";
  const iss = payload.iss ?? "https://test-clerk.accounts.dev";
  const aud = payload.aud ?? "test_audience";

  const fullPayload = {
    sub: payload.sub,
    azp: payload.azp,
    exp,
    iat,
    nbf,
    sid,
    iss,
    aud,
  };

  const token = await signJwt(fullPayload, privateJwk, {
    algorithm: "RS256",
    header: { typ: payload.typ ?? "JWT" },
  });
  if (!token || typeof token !== "string") {
    throw new Error("Failed to sign test JWT");
  }
  return token;
}

/** Build a Worker `Request` with a Bearer authorization header. */
export function buildAuthenticatedRequest(
  url: string,
  token: string,
  extra?: { method?: string; body?: string },
): Request {
  const headers = new Headers({
    Authorization: `Bearer ${token}`,
    Origin: new URL(url).origin,
  });
  return new Request(url, {
    method: extra?.method ?? "GET",
    headers,
    body: extra?.body,
  });
}

/** Build a request with NO session cookie (unauthenticated). */
export function buildUnauthenticatedRequest(url: string): Request {
  return new Request(url, { method: "GET" });
}

/** Real browser cookie carrier; no Authorization header. Companion cookies
 * follow @clerk/backend 3.22's development-instance cookie eligibility checks.
 */
export function buildCookieRequest(url: string, token: string, document = false): Request {
  return new Request(url, {
    headers: {
      Cookie: `__session=${token}; __client_uat=1; __clerk_db_jwt=test_dev_browser`,
      ...(document ? { "Sec-Fetch-Dest": "document", Accept: "text/html" } : {}),
    },
  });
}
