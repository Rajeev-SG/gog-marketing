/**
 * AES-256-GCM encryption of Google refresh credentials at rest.
 *
 * Root key material comes from Worker secrets only:
 * - `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY`: standard base64 of 32 random
 *   bytes, provisioned as key_version 1.
 * - `GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEYS` (optional rotation): JSON map of
 *   `{"<version>": "<base64 32B>"}`; encryption always uses the highest
 *   version and decryption looks up the stored `key_version`.
 *
 * Every ciphertext is bound to the tenant, connection, and key version via
 * AES-GCM additional authenticated data, so a ciphertext moved between
 * connections fails authentication instead of decrypting.
 */

export interface CredentialKeySet {
  /** version string -> raw 32-byte key */
  keys: Map<string, Uint8Array>;
  /** highest numeric version used for new ciphertext */
  latestVersion: string;
}

export class CredentialKeyError extends Error {}

export function parseCredentialKeys(singleKey?: string, rotatedKeys?: string): CredentialKeySet {
  const keys = new Map<string, Uint8Array>();
  if (rotatedKeys && rotatedKeys.trim()) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(rotatedKeys);
    } catch {
      throw new CredentialKeyError("credential encryption keys configuration is invalid");
    }
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new CredentialKeyError("credential encryption keys configuration is invalid");
    }
    for (const [version, value] of Object.entries(parsed as Record<string, unknown>)) {
      if (!/^\d+$/.test(version)) {
        throw new CredentialKeyError("credential encryption keys configuration is invalid");
      }
      keys.set(version, decodeKey(value));
    }
  }
  if (singleKey && singleKey.trim()) {
    if (!keys.has("1")) keys.set("1", decodeKey(singleKey));
  }
  if (keys.size === 0) {
    throw new CredentialKeyError("credential encryption key is missing");
  }
  const versions = [...keys.keys()].map(Number).sort((a, b) => b - a);
  return { keys, latestVersion: String(versions[0]) };
}

function decodeKey(value: unknown): Uint8Array {
  if (typeof value !== "string") {
    throw new CredentialKeyError("credential encryption key is invalid");
  }
  let bytes: Uint8Array;
  try {
    const binary = atob(value.trim());
    bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  } catch {
    throw new CredentialKeyError("credential encryption key is invalid");
  }
  if (bytes.length !== 32) {
    throw new CredentialKeyError("credential encryption key must be 32 bytes");
  }
  return bytes;
}

/** Payload shape mirrors the Go control-plane's encrypted token JSON. */
export interface StoredGoogleCredential {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expiry: string;
  granted_scopes: string[];
}

export interface EncryptedCredential {
  ciphertext: Uint8Array;
  nonce: Uint8Array;
  keyVersion: string;
}

const AAD_PREFIX = "gog-hosted-v1/google-credential";

function aadFor(tenantId: string, connectionId: string, keyVersion: string): Uint8Array {
  return new TextEncoder().encode(`${AAD_PREFIX}/${tenantId}/${connectionId}/${keyVersion}`);
}

export class CredentialCipher {
  private readonly keySet: CredentialKeySet;
  private readonly imported = new Map<string, CryptoKey>();

  constructor(keySet: CredentialKeySet) {
    this.keySet = keySet;
  }

  private async cryptoKeyFor(version: string): Promise<CryptoKey> {
    const cached = this.imported.get(version);
    if (cached) return cached;
    const raw = this.keySet.keys.get(version);
    if (!raw) throw new CredentialKeyError("credential encryption key version is unknown");
    const key = await crypto.subtle.importKey(
      "raw",
      raw as BufferSource,
      { name: "AES-GCM" },
      false,
      ["encrypt", "decrypt"],
    );
    this.imported.set(version, key);
    return key;
  }

  async encrypt(
    tenantId: string,
    connectionId: string,
    credential: StoredGoogleCredential,
  ): Promise<EncryptedCredential> {
    const keyVersion = this.keySet.latestVersion;
    const key = await this.cryptoKeyFor(keyVersion);
    const nonce = crypto.getRandomValues(new Uint8Array(12));
    const plaintext = new TextEncoder().encode(JSON.stringify(credential));
    const ciphertext = await crypto.subtle.encrypt(
      {
        name: "AES-GCM",
        iv: nonce as BufferSource,
        additionalData: aadFor(tenantId, connectionId, keyVersion) as BufferSource,
      },
      key,
      plaintext as BufferSource,
    );
    return { ciphertext: new Uint8Array(ciphertext), nonce, keyVersion };
  }

  async decrypt(
    tenantId: string,
    connectionId: string,
    input: { ciphertext: Uint8Array; nonce: Uint8Array; keyVersion: number | string },
  ): Promise<StoredGoogleCredential> {
    const keyVersion = String(input.keyVersion);
    const key = await this.cryptoKeyFor(keyVersion);
    let plaintext: ArrayBuffer;
    try {
      plaintext = await crypto.subtle.decrypt(
        {
          name: "AES-GCM",
          iv: input.nonce as BufferSource,
          additionalData: aadFor(tenantId, connectionId, keyVersion) as BufferSource,
        },
        key,
        input.ciphertext as BufferSource,
      );
    } catch {
      throw new CredentialKeyError("stored credential could not be authenticated");
    }
    try {
      const parsed = JSON.parse(new TextDecoder().decode(plaintext)) as StoredGoogleCredential;
      if (typeof parsed.access_token !== "string" || typeof parsed.refresh_token !== "string") {
        throw new Error("shape");
      }
      return {
        access_token: parsed.access_token,
        refresh_token: parsed.refresh_token,
        token_type: String(parsed.token_type ?? "Bearer"),
        expiry: String(parsed.expiry ?? ""),
        granted_scopes: Array.isArray(parsed.granted_scopes)
          ? parsed.granted_scopes.map(String)
          : [],
      };
    } catch {
      throw new CredentialKeyError("stored credential payload is invalid");
    }
  }
}
