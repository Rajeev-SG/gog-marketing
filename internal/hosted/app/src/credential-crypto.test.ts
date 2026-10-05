import { describe, expect, it } from "vitest";
import {
  CredentialCipher,
  CredentialKeyError,
  parseCredentialKeys,
  type StoredGoogleCredential,
} from "./credential-crypto.js";

const KEY_V1 = Buffer.alloc(32, 1).toString("base64");
const KEY_V2 = Buffer.alloc(32, 2).toString("base64");

const credential: StoredGoogleCredential = {
  access_token: "ya29.access",
  refresh_token: "1//refresh",
  token_type: "Bearer",
  expiry: "2026-10-05T01:00:00.000Z",
  granted_scopes: ["openid", "email"],
};

describe("credential encryption at rest (#62)", () => {
  it("parses the documented single-key contract as key_version 1", () => {
    const keys = parseCredentialKeys(KEY_V1);
    expect(keys.latestVersion).toBe("1");
    expect(keys.keys.get("1")?.length).toBe(32);
  });

  it("rejects missing, malformed, or wrong-size keys", () => {
    expect(() => parseCredentialKeys()).toThrow(CredentialKeyError);
    expect(() => parseCredentialKeys("not-base64!!")).toThrow(CredentialKeyError);
    expect(() => parseCredentialKeys(Buffer.alloc(16).toString("base64"))).toThrow(
      CredentialKeyError,
    );
    expect(() => parseCredentialKeys(undefined, "{bad json")).toThrow(CredentialKeyError);
    expect(() => parseCredentialKeys(undefined, '{"x":"<b64>"}')).toThrow(CredentialKeyError);
  });

  it("round-trips through AES-GCM and never stores plaintext", async () => {
    const cipher = new CredentialCipher(parseCredentialKeys(KEY_V1));
    const encrypted = await cipher.encrypt("tenant-1", "conn-1", credential);
    const text = new TextDecoder().decode(encrypted.ciphertext);
    expect(text).not.toContain("1//refresh");
    expect(text).not.toContain("ya29.access");
    expect(encrypted.nonce.length).toBe(12);
    expect(encrypted.keyVersion).toBe("1");
    const decrypted = await cipher.decrypt("tenant-1", "conn-1", {
      ciphertext: encrypted.ciphertext,
      nonce: encrypted.nonce,
      keyVersion: encrypted.keyVersion,
    });
    expect(decrypted).toEqual(credential);
  });

  it("binds ciphertext to tenant + connection via AAD", async () => {
    const cipher = new CredentialCipher(parseCredentialKeys(KEY_V1));
    const encrypted = await cipher.encrypt("tenant-1", "conn-1", credential);
    await expect(
      cipher.decrypt("tenant-1", "conn-2", {
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: encrypted.keyVersion,
      }),
    ).rejects.toThrow(CredentialKeyError);
    await expect(
      cipher.decrypt("tenant-2", "conn-1", {
        ciphertext: encrypted.ciphertext,
        nonce: encrypted.nonce,
        keyVersion: encrypted.keyVersion,
      }),
    ).rejects.toThrow(CredentialKeyError);
  });

  it("encrypts with the highest rotation version and decrypts by stored version", async () => {
    const rotated = parseCredentialKeys(undefined, JSON.stringify({ "1": KEY_V1, "2": KEY_V2 }));
    expect(rotated.latestVersion).toBe("2");
    const cipher = new CredentialCipher(rotated);
    const encrypted = await cipher.encrypt("tenant-1", "conn-1", credential);
    expect(encrypted.keyVersion).toBe("2");

    const v1Only = new CredentialCipher(parseCredentialKeys(KEY_V1));
    const legacy = await v1Only.encrypt("tenant-1", "conn-1", credential);
    expect(legacy.keyVersion).toBe("1");
    // Rotation map still understands older versions.
    const decrypted = await cipher.decrypt("tenant-1", "conn-1", legacy);
    expect(decrypted).toEqual(credential);

    // Unknown versions fail closed.
    await expect(
      new CredentialCipher(parseCredentialKeys(KEY_V2)).decrypt("tenant-1", "conn-1", legacy),
    ).rejects.toThrow(CredentialKeyError);
  });
});
