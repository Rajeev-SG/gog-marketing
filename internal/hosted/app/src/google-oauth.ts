/**
 * Google OAuth control-plane client for the hosted Worker.
 *
 * Mirrors the Go control-plane's `GoogleOAuthProvider` (PKCE S256, offline
 * access, OIDC nonce, userinfo identity) over Web-standard fetch. Endpoint
 * URLs default to Google's canonical endpoints and may be overridden through
 * non-secret env vars for staging and tests.
 *
 * Failures are classified into distinct, user-facing kinds; raw provider
 * exception text is never propagated to responses.
 */

export type GoogleOAuthFailureKind =
  | "consent_denied"
  | "invalid_grant"
  | "invalid_request"
  | "api_error"
  | "network"
  | "identity_failed"
  | "nonce_mismatch"
  | "config_missing";

export class GoogleOAuthError extends Error {
  readonly kind: GoogleOAuthFailureKind;

  constructor(kind: GoogleOAuthFailureKind, message: string) {
    super(message);
    this.name = "GoogleOAuthError";
    this.kind = kind;
  }
}

export interface GoogleEndpoints {
  authUrl: string;
  tokenUrl: string;
  userinfoUrl: string;
  revokeUrl: string;
}

export const DEFAULT_GOOGLE_ENDPOINTS: GoogleEndpoints = {
  authUrl: "https://accounts.google.com/o/oauth2/v2/auth",
  tokenUrl: "https://oauth2.googleapis.com/token",
  userinfoUrl: "https://openidconnect.googleapis.com/v1/userinfo",
  revokeUrl: "https://oauth2.googleapis.com/revoke",
};

export interface AuthorizationRequest {
  state: string;
  codeVerifier: string;
  nonce: string;
  scopes: string[];
  redirectUri: string;
  forceConsent?: boolean;
}

export interface GoogleTokenSet {
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  /** ISO timestamp. */
  expiry: string;
  grantedScopes: string[];
  idToken: string;
}

export interface GoogleIdentity {
  subject: string;
  email: string;
  emailVerified: boolean;
  displayName: string;
}

interface TokenResponse {
  access_token?: unknown;
  refresh_token?: unknown;
  token_type?: unknown;
  expires_in?: unknown;
  scope?: unknown;
  id_token?: unknown;
  error?: unknown;
}

interface UserinfoResponse {
  sub?: unknown;
  email?: unknown;
  email_verified?: unknown;
  name?: unknown;
}

export class GoogleOAuthClient {
  private readonly clientId: string;
  private readonly clientSecret: string;
  readonly endpoints: GoogleEndpoints;
  private readonly fetchImpl: typeof fetch;

  constructor(options: {
    clientId: string;
    clientSecret: string;
    endpoints?: Partial<GoogleEndpoints>;
    fetchImpl?: typeof fetch;
  }) {
    this.clientId = options.clientId;
    this.clientSecret = options.clientSecret;
    // Explicit `undefined` values must not clobber the canonical defaults.
    this.endpoints = { ...DEFAULT_GOOGLE_ENDPOINTS };
    if (options.endpoints) {
      for (const [key, value] of Object.entries(options.endpoints)) {
        if (value) (this.endpoints as unknown as Record<string, string>)[key] = value;
      }
    }
    this.fetchImpl = options.fetchImpl ?? fetch.bind(globalThis);
  }

  async authorizationURL(request: AuthorizationRequest): Promise<string> {
    const url = new URL(this.endpoints.authUrl);
    const challengeBuffer = await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(request.codeVerifier),
    );
    const challenge = base64UrlEncode(new Uint8Array(challengeBuffer));
    url.searchParams.set("client_id", this.clientId);
    url.searchParams.set("redirect_uri", request.redirectUri);
    url.searchParams.set("response_type", "code");
    url.searchParams.set("scope", request.scopes.join(" "));
    url.searchParams.set("access_type", "offline");
    // Incremental authorization: previously granted scopes are combined with
    // the requested set rather than replaced (Google web-server flow docs).
    url.searchParams.set("include_granted_scopes", "true");
    url.searchParams.set("prompt", request.forceConsent === false ? "" : "consent");
    if (url.searchParams.get("prompt") === "") url.searchParams.delete("prompt");
    url.searchParams.set("state", request.state);
    url.searchParams.set("nonce", request.nonce);
    url.searchParams.set("code_challenge", challenge);
    url.searchParams.set("code_challenge_method", "S256");
    return url.toString();
  }

  async exchangeCode(input: {
    code: string;
    codeVerifier: string;
    nonce: string;
    redirectUri: string;
  }): Promise<GoogleTokenSet> {
    const token = await this.postTokenForm(
      new URLSearchParams({
        grant_type: "authorization_code",
        code: input.code,
        code_verifier: input.codeVerifier,
        client_id: this.clientId,
        client_secret: this.clientSecret,
        redirect_uri: input.redirectUri,
      }),
    );
    await this.assertNonce(token, input.nonce);
    return token;
  }

  async refreshTokenSet(input: {
    refreshToken: string;
    previous: GoogleTokenSet;
  }): Promise<GoogleTokenSet> {
    const token = await this.postTokenForm(
      new URLSearchParams({
        grant_type: "refresh_token",
        refresh_token: input.refreshToken,
        client_id: this.clientId,
        client_secret: this.clientSecret,
      }),
    );
    if (!token.refreshToken) token.refreshToken = input.refreshToken;
    if (!token.grantedScopes.length) token.grantedScopes = input.previous.grantedScopes;
    return token;
  }

  async revoke(token: string): Promise<void> {
    const value = token.trim();
    if (!value) return;
    let response: Response;
    try {
      response = await this.fetchImpl(this.endpoints.revokeUrl, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ token: value }).toString(),
        signal: AbortSignal.timeout(10_000),
      });
    } catch {
      throw new GoogleOAuthError("network", "Google token revoke is unreachable");
    }
    // 400 = already revoked/invalid: revocation is idempotent for us.
    if (response.status >= 300 && response.status !== 400) {
      throw new GoogleOAuthError("api_error", "Google token revoke failed");
    }
  }

  /**
   * Verify the returned Google identity through the userinfo endpoint using
   * the exchanged access token. This is the identity authority before any
   * connection is created or replaced: a missing subject or an unverified
   * email is rejected.
   */
  async identity(accessToken: string): Promise<GoogleIdentity> {
    let response: Response;
    try {
      response = await this.fetchImpl(this.endpoints.userinfoUrl, {
        headers: { Authorization: `Bearer ${accessToken}` },
        signal: AbortSignal.timeout(10_000),
      });
    } catch {
      throw new GoogleOAuthError("network", "Google identity endpoint is unreachable");
    }
    if (response.status < 200 || response.status >= 300) {
      throw new GoogleOAuthError("identity_failed", "Google identity could not be verified");
    }
    let body: UserinfoResponse;
    try {
      body = (await response.json()) as UserinfoResponse;
    } catch {
      throw new GoogleOAuthError("identity_failed", "Google identity could not be verified");
    }
    const subject = String(body.sub ?? "").trim();
    if (!subject) {
      throw new GoogleOAuthError("identity_failed", "Google identity is missing subject");
    }
    const email = String(body.email ?? "").trim();
    const emailVerified = body.email_verified === true;
    if (email && !emailVerified) {
      throw new GoogleOAuthError("identity_failed", "Google email is not verified");
    }
    return {
      subject,
      email,
      emailVerified,
      displayName: String(body.name ?? "").trim(),
    };
  }

  /**
   * Defense-in-depth check of the OIDC id_token issued with the exchange:
   * audience must be this client and the nonce must match the flow binding.
   * (Signature verification is not performed here; the userinfo call above,
   * reached with the TLS-exchanged access token, is the identity authority.)
   */
  async assertNonce(token: GoogleTokenSet, expectedNonce: string): Promise<void> {
    if (!token.idToken || !expectedNonce) return;
    const parts = token.idToken.split(".");
    if (parts.length < 2) {
      throw new GoogleOAuthError("nonce_mismatch", "Google OIDC nonce could not be checked");
    }
    let claims: { nonce?: unknown; aud?: unknown };
    try {
      claims = JSON.parse(new TextDecoder().decode(base64UrlDecode(parts[1]!)));
    } catch {
      throw new GoogleOAuthError("nonce_mismatch", "Google OIDC nonce could not be checked");
    }
    if (
      String(claims.nonce ?? "") !== expectedNonce ||
      String(claims.aud ?? "") !== this.clientId
    ) {
      throw new GoogleOAuthError("nonce_mismatch", "Google OIDC nonce mismatch");
    }
  }

  private async postTokenForm(form: URLSearchParams): Promise<GoogleTokenSet> {
    let response: Response;
    try {
      response = await this.fetchImpl(this.endpoints.tokenUrl, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: form.toString(),
        signal: AbortSignal.timeout(15_000),
      });
    } catch {
      throw new GoogleOAuthError("network", "Google OAuth token endpoint is unreachable");
    }
    let body: TokenResponse;
    try {
      body = (await response.json()) as TokenResponse;
    } catch {
      throw new GoogleOAuthError(
        response.status >= 500 ? "api_error" : "invalid_request",
        "Google OAuth token response is malformed",
      );
    }
    if (!response.ok || body.error) {
      const code = String(body.error ?? "").trim();
      const lowered = code.toLowerCase();
      if (lowered === "access_denied") {
        throw new GoogleOAuthError("consent_denied", "Google consent was denied");
      }
      if (lowered === "invalid_client" || lowered === "deleted_client") {
        throw new GoogleOAuthError("config_missing", "Google OAuth operator configuration failed");
      }
      if (lowered === "invalid_grant") {
        throw new GoogleOAuthError("invalid_grant", "Google authorization expired or was revoked");
      }
      if (response.status >= 500) {
        throw new GoogleOAuthError("api_error", "Google OAuth service failed");
      }
      throw new GoogleOAuthError("invalid_request", "Google OAuth request was rejected");
    }
    const accessToken = String(body.access_token ?? "").trim();
    if (!accessToken) {
      throw new GoogleOAuthError(
        "api_error",
        "Google OAuth token response is missing access token",
      );
    }
    const expiresIn = Number(body.expires_in ?? 0);
    return {
      accessToken,
      refreshToken: String(body.refresh_token ?? "").trim(),
      tokenType: String(body.token_type ?? "Bearer").trim() || "Bearer",
      expiry: new Date(
        Date.now() + (Number.isFinite(expiresIn) ? expiresIn : 0) * 1000,
      ).toISOString(),
      grantedScopes: String(body.scope ?? "")
        .split(/\s+/)
        .map((s) => s.trim())
        .filter(Boolean),
      idToken: String(body.id_token ?? "").trim(),
    };
  }
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

function base64UrlDecode(value: string): Uint8Array {
  const normalized = value.replaceAll("-", "+").replaceAll("_", "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
  const binary = atob(padded);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}
