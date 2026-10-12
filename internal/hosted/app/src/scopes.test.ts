import { describe, expect, it } from "vitest";
import {
  IDENTITY_SCOPES,
  HOSTED_SERVICES,
  isHostedService,
  mergeScopes,
  normalizeServices,
  scopesForServices,
  UnknownServiceError,
  assertHostedScopes,
  UnsafeHostedScopeError,
} from "./scopes.js";

describe("hosted Google scope catalog (#62)", () => {
  it("covers exactly the 23 v1 services and excludes owner-disabled services", () => {
    expect(Object.keys(HOSTED_SERVICES).sort()).toEqual([
      "adsense",
      "analytics",
      "appscript",
      "bigquery",
      "calendar",
      "chat",
      "classroom",
      "contacts",
      "docs",
      "drive",
      "drivelabels",
      "forms",
      "groups",
      "meet",
      "people",
      "photos",
      "searchconsole",
      "sheets",
      "sites",
      "slides",
      "tagmanager",
      "tasks",
      "youtube",
    ]);
    for (const service of [
      "gmail",
      "googleads",
      "driveactivity",
      "admin",
      "cloudadmin",
      "keep",
      "photospicker",
      "constructor",
      "__proto__",
    ]) {
      expect(isHostedService(service)).toBe(false);
      expect(() => scopesForServices([service])).toThrow(UnknownServiceError);
    }
  });

  it("requests write scopes only for explicit selected write services", () => {
    const read = scopesForServices(["analytics", "tagmanager", "youtube"]);
    expect(read).not.toContain("https://www.googleapis.com/auth/analytics.edit");
    const write = scopesForServices(
      ["analytics", "tagmanager", "youtube"],
      ["analytics", "tagmanager", "youtube"],
    );
    expect(write).toContain("https://www.googleapis.com/auth/analytics.edit");
    expect(write).toContain("https://www.googleapis.com/auth/tagmanager.publish");
    expect(write).toContain("https://www.googleapis.com/auth/youtube");
    expect(() => scopesForServices(["analytics"], ["calendar"])).toThrow(UnknownServiceError);
    expect(() => scopesForServices(["adsense"], ["adsense"])).toThrow(UnknownServiceError);
  });

  it("never introduces restricted dependencies through any service or write combination", () => {
    const names = Object.keys(HOSTED_SERVICES);
    const writes = names.filter((name) => HOSTED_SERVICES[name]!.writeScopes.length);
    const all = scopesForServices(names, writes);
    expect(all).toContain("https://www.googleapis.com/auth/drive.file");
    for (const name of [
      "drive",
      "drive.readonly",
      "drive.metadata",
      "drive.metadata.readonly",
      "drive.activity.readonly",
      "chat.messages",
      "chat.messages.readonly",
      "adwords",
      "cloud-platform",
      "keep",
      "admin.directory.user",
      "photospicker.mediaitems.readonly",
      "gmail.readonly",
      "gmail.send",
      "mail.google.com",
    ]) {
      expect(all).not.toContain("https://www.googleapis.com/auth/" + name);
      expect(() => assertHostedScopes(["https://www.googleapis.com/auth/" + name])).toThrow(
        UnsafeHostedScopeError,
      );
      expect(() => mergeScopes(["openid"], ["https://www.googleapis.com/auth/" + name])).toThrow(
        UnsafeHostedScopeError,
      );
    }
    expect(() => assertHostedScopes(["https://mail.google.com/"])).toThrow(UnsafeHostedScopeError);
    expect(() => assertHostedScopes(["https://www.googleapis.com/auth/new.unknown.scope"])).toThrow(
      UnsafeHostedScopeError,
    );
    expect(() => assertHostedScopes([])).toThrow(UnsafeHostedScopeError);
  });

  it("always includes identity scopes for verified identity", () => {
    const scopes = scopesForServices(["analytics"]);
    for (const identityScope of IDENTITY_SCOPES) {
      expect(scopes).toContain(identityScope);
    }
    expect(scopes).toContain("https://www.googleapis.com/auth/analytics.readonly");
  });

  it("supports identity-only connects with no service scopes", () => {
    expect(scopesForServices([])).toEqual([...IDENTITY_SCOPES].sort());
  });

  it("rejects unknown services", () => {
    expect(() => scopesForServices(["not-a-service"])).toThrow(UnknownServiceError);
    expect(() => normalizeServices(["not-a-service"])).toThrow(UnknownServiceError);
    expect(() => normalizeServices("analytics")).toThrow(UnknownServiceError);
    expect(isHostedService("analytics")).toBe(true);
    expect(isHostedService("youtube")).toBe(true);
  });

  it("normalizes and dedupes services case-insensitively", () => {
    expect(normalizeServices(["Analytics", " analytics ", "calendar"])).toEqual([
      "analytics",
      "calendar",
    ]);
    expect(normalizeServices(undefined)).toEqual([]);
  });

  it("merges granted scopes sorted and deduplicated", () => {
    expect(
      mergeScopes(
        ["openid"],
        ["email", "openid", " https://www.googleapis.com/auth/analytics.readonly "],
      ),
    ).toEqual(["email", "https://www.googleapis.com/auth/analytics.readonly", "openid"]);
  });
});
