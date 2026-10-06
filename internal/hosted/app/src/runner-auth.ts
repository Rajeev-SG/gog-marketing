/**
 * Operator-owned identity material for the private Go discovery runner.
 *
 * This module never handles tenant Google credentials: the WIF class mints
 * an operator-scoped Cloud Run identity token, while the capability signer
 * binds an application-level tenant/connection/operation snapshot to exact
 * request bytes. Provider endpoints are fixed constants and cannot be
 * influenced by caller input.
 */

const WORKER_IDENTITY = "gog-marketing-worker";
const RUNNER_IDENTITY = "gog-marketing-runner";
const STS_TOKEN_URL = "https://sts.googleapis.com/v1/token";
const IAM_CREDENTIALS_HOST = "https://iamcredentials.googleapis.com";
const NATIVE_ID_CACHE_SKEW_MS = 60_000;

/** Allowed signed contexts, mirroring the Go request's fixed operations. */
export type RunnerOperation = "discover" | "analytics_property_read";

/** Exact WIF configuration supplied by the trusted operator environment. */
export interface RunnerIdentityConfig {
  provider: string;
  issuer: string;
  keyId: string;
  /** Standard base64 PKCS8 DER for the dedicated Worker RSA2048 key. */
  signingKey: string;
  serviceAccount: string;
}

/** Worker-facing native identity seam. Native tokens are the only cached values. */
export interface NativeIdentityProvider {
  nativeIdToken(runnerOrigin: string, signal?: AbortSignal): Promise<string>;
}

type CacheEntry = { token: string; expiresAtMs: number };

const nativeIdentityCache = new Map<string, CacheEntry>();

/** Test-only lifecycle hook; never exposes cached tokens outside the process. */
export function clearNativeIdentityCacheForTest(): void {
  nativeIdentityCache.clear();
}

export function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

function base64UrlDecode(value: string): Uint8Array {
  let normalized = value.replaceAll("-", "+").replaceAll("_", "/");
  while (normalized.length % 4 !== 0) normalized += "=";
  const binary = atob(normalized);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function utf8Bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value);
}

async function signCapability(secret: string, signedInput: string): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    utf8Bytes(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign("HMAC", key, utf8Bytes(signedInput));
  return base64UrlEncode(new Uint8Array(signature));
}

/**
 * Signs the exact Go application capability. The caller must pass the raw
 * UTF-8 body bytes; using the subsequently serialized HTTP request object
 * is not sufficient because JSON representation differences would invalidate
 * the Go-side digest.
 */
export async function signRunnerCapabilityJwt(
  invocationToken: string,
  context: {
    tenantId: string;
    connectionId: string;
    operation: RunnerOperation;
  },
  exactJsonBody: Uint8Array,
  now = new Date(),
): Promise<string> {
  const secretBytes = utf8Bytes(invocationToken);
  if (secretBytes.length < 32) {
    throw new Error("invocation secret is too short");
  }
  const issuedAt = Math.floor(now.getTime() / 1000);
  const expiresAt = issuedAt + 55;
  const digest = await crypto.subtle.digest("SHA-256", exactJsonBody);
  const claims = {
    iss: WORKER_IDENTITY,
    sub: WORKER_IDENTITY,
    aud: RUNNER_IDENTITY,
    iat: issuedAt,
    exp: expiresAt,
    tenant_id: context.tenantId,
    connection_id: context.connectionId,
    operation: context.operation,
    request_sha256: [...new Uint8Array(digest)]
      .map((byte) => byte.toString(16).padStart(2, "0"))
      .join(""),
  };
  const encodedHeader = base64UrlEncode(utf8Bytes(JSON.stringify({ alg: "HS256", typ: "JWT" })));
  const encodedPayload = base64UrlEncode(utf8Bytes(JSON.stringify(claims)));
  const signingInput = `${encodedHeader}.${encodedPayload}`;
  const signature = await signCapability(invocationToken, signingInput);
  return `${signingInput}.${signature}`;
}

export function validateRunnerIdentityConfig(config: RunnerIdentityConfig): URL {
  const provider = config.provider.trim();
  const issuerRaw = config.issuer.trim();
  const keyId = config.keyId.trim();
  const serviceAccount = config.serviceAccount.trim();
  const providerPattern =
    /^projects\/[0-9]+\/locations\/global\/workloadIdentityPools\/[A-Za-z0-9_.-]+\/providers\/[A-Za-z0-9_.-]+$/;
  if (!providerPattern.test(provider)) {
    throw new Error("identity provider is not configured");
  }
  let issuer: URL;
  try {
    issuer = new URL(issuerRaw);
  } catch {
    throw new Error("identity issuer is not configured");
  }
  if (issuer.protocol !== "https:" || issuer.pathname !== "/") {
    throw new Error("identity issuer is not configured");
  }
  if (!keyId) {
    throw new Error("identity key is not configured");
  }
  if (!/^[a-z0-9.-]+@[-a-z0-9.]+\.[a-z]{2,}$/i.test(serviceAccount)) {
    throw new Error("service account is not configured");
  }
  try {
    const keyBytes = base64ToBytes(config.signingKey.trim());
    if (keyBytes.length < 200) throw new Error("identity key is not configured");
  } catch {
    throw new Error("identity key is not configured");
  }
  return issuer;
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function cacheKey(config: RunnerIdentityConfig, runnerOrigin: string): string {
  return [
    config.provider.trim(),
    config.issuer.trim(),
    config.keyId.trim(),
    config.serviceAccount.trim(),
    runnerOrigin,
  ].join("\n");
}

function decodeIdTokenExpiry(token: string): number {
  const parts = token.split(".");
  if (parts.length !== 3 || !parts[0] || !parts[1] || !parts[2]) {
    throw new Error("identity response is invalid");
  }
  let payload: { exp?: unknown };
  try {
    payload = JSON.parse(new TextDecoder().decode(base64UrlDecode(parts[1]!))) as {
      exp?: unknown;
    };
  } catch {
    throw new Error("identity response is invalid");
  }
  const expiry = payload.exp;
  if (typeof expiry !== "number" || !Number.isInteger(expiry) || expiry <= 0) {
    throw new Error("identity response is invalid");
  }
  return expiry * 1000;
}

/**
 * Performs the two documented, fixed WIF calls and caches only the resulting
 * native Cloud Run ID token. The returned token is handed only to a private
 * HTTPS runner request and never logged or returned to a browser.
 */
export class WorkloadIdentityClient implements NativeIdentityProvider {
  readonly #config: RunnerIdentityConfig;
  readonly #privateKey: Promise<CryptoKey>;
  readonly #fetchImpl: typeof fetch;
  readonly #now: () => Date;

  constructor(options: {
    config: RunnerIdentityConfig;
    fetchImpl?: typeof fetch;
    now?: () => Date;
  }) {
    validateRunnerIdentityConfig(options.config);
    this.#config = {
      ...options.config,
      provider: options.config.provider.trim(),
      issuer: options.config.issuer.trim(),
      keyId: options.config.keyId.trim(),
      serviceAccount: options.config.serviceAccount.trim(),
      signingKey: options.config.signingKey.trim(),
    };
    this.#privateKey = crypto.subtle.importKey(
      "pkcs8",
      base64ToBytes(this.#config.signingKey),
      {
        name: "RSASSA-PKCS1-v1_5",
        hash: "SHA-256",
      },
      false,
      ["sign"],
    );
    this.#fetchImpl = options.fetchImpl ?? fetch.bind(globalThis);
    this.#now = options.now ?? (() => new Date(Date.now()));
  }

  async nativeIdToken(runnerOrigin: string, signal?: AbortSignal): Promise<string> {
    const origin = new URL(runnerOrigin).origin;
    const now = this.#now();
    const key = cacheKey(this.#config, origin);
    const cached = nativeIdentityCache.get(key);
    if (cached && cached.expiresAtMs - NATIVE_ID_CACHE_SKEW_MS > now.getTime()) {
      return cached.token;
    }
    nativeIdentityCache.delete(key);

    const callerJwt = await this.#signCallerJwt(now);
    const accessResponse = await this.#postJson(
      STS_TOKEN_URL,
      {
        grantType: "urn:ietf:params:oauth:grant-type:token-exchange",
        audience: `//iam.googleapis.com/${this.#config.provider}`,
        scope: "https://www.googleapis.com/auth/cloud-platform",
        requestedTokenType: "urn:ietf:params:oauth:token-type:access_token",
        subjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
        subjectToken: callerJwt,
      },
      signal,
    );
    if (!accessResponse.ok) {
      throw new Error("identity exchange failed");
    }
    const accessBody = (await accessResponse.json()) as { access_token?: unknown };
    const accessToken = accessBody.access_token;
    if (typeof accessToken !== "string" || !accessToken) {
      throw new Error("identity exchange failed");
    }

    const idResponse = await this.#postJson(
      `${IAM_CREDENTIALS_HOST}/v1/projects/-/serviceAccounts/${encodeURIComponent(
        this.#config.serviceAccount,
      )}:generateIdToken`,
      { audience: origin, includeEmail: true },
      signal,
      `Bearer ${accessToken}`,
    );
    if (!idResponse.ok) {
      throw new Error("identity token failed");
    }
    const idBody = (await idResponse.json()) as { token?: unknown };
    const idToken = idBody.token;
    if (typeof idToken !== "string" || !idToken) {
      throw new Error("identity token failed");
    }
    const expiresAtMs = decodeIdTokenExpiry(idToken);
    if (expiresAtMs > now.getTime() + NATIVE_ID_CACHE_SKEW_MS) {
      nativeIdentityCache.set(key, { token: idToken, expiresAtMs });
    }
    return idToken;
  }

  async #signCallerJwt(now: Date): Promise<string> {
    const issuedAt = Math.floor(now.getTime() / 1000);
    const header = base64UrlEncode(
      utf8Bytes(JSON.stringify({ alg: "RS256", typ: "JWT", kid: this.#config.keyId })),
    );
    const payload = base64UrlEncode(
      utf8Bytes(
        JSON.stringify({
          iss: this.#config.issuer,
          sub: WORKER_IDENTITY,
          aud: `https://iam.googleapis.com/${this.#config.provider}`,
          iat: issuedAt,
          exp: issuedAt + 300,
        }),
      ),
    );
    const signature = await crypto.subtle.sign(
      "RSASSA-PKCS1-v1_5",
      await this.#privateKey,
      utf8Bytes(`${header}.${payload}`),
    );
    return `${header}.${payload}.${base64UrlEncode(new Uint8Array(signature))}`;
  }

  async #postJson(
    url: string,
    body: Record<string, unknown>,
    signal: AbortSignal | undefined,
    authorization?: string,
  ): Promise<Response> {
    return await this.#fetchImpl(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(authorization ? { Authorization: authorization } : {}),
      },
      body: JSON.stringify(body),
      signal,
    });
  }
}
