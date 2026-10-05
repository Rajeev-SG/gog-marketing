import { describe, expect, it } from "vitest";
import {
  IDENTITY_SCOPES,
  HOSTED_SERVICES,
  isHostedService,
  mergeScopes,
  normalizeServices,
  scopesForServices,
  UnknownServiceError,
} from "./scopes.js";

describe("hosted Google scope catalog (#62)", () => {
  it("requests read-only scopes only for the hosted service surface", () => {
    expect(HOSTED_SERVICES.analytics.scopes).toEqual([
      "https://www.googleapis.com/auth/analytics.readonly",
    ]);
    expect(HOSTED_SERVICES.gmail.scopes).toEqual([
      "https://www.googleapis.com/auth/gmail.readonly",
    ]);
    expect(HOSTED_SERVICES.drive.scopes).toEqual([
      "https://www.googleapis.com/auth/drive.readonly",
    ]);
    expect(HOSTED_SERVICES.calendar.scopes).toEqual([
      "https://www.googleapis.com/auth/calendar.readonly",
    ]);
    expect(HOSTED_SERVICES.tagmanager.scopes).toEqual([
      "https://www.googleapis.com/auth/tagmanager.readonly",
    ]);
    expect(HOSTED_SERVICES.searchconsole.scopes).toEqual([
      "https://www.googleapis.com/auth/webmasters.readonly",
    ]);
    expect(HOSTED_SERVICES.bigquery.scopes).toEqual([
      "https://www.googleapis.com/auth/bigquery.readonly",
    ]);
    expect(HOSTED_SERVICES.googleads.scopes).toEqual(["https://www.googleapis.com/auth/adwords"]);
  });

  it("never offers write scopes", () => {
    const allScopes = Object.values(HOSTED_SERVICES).flatMap((s) => s.scopes);
    const forbidden = [
      "https://www.googleapis.com/auth/gmail.modify",
      "https://www.googleapis.com/auth/gmail.send",
      "https://www.googleapis.com/auth/gmail.settings.basic",
      "https://www.googleapis.com/auth/drive",
      "https://www.googleapis.com/auth/calendar",
      "https://www.googleapis.com/auth/analytics.edit",
      "https://www.googleapis.com/auth/bigquery",
      "https://www.googleapis.com/auth/tagmanager.edit.containers",
      "https://www.googleapis.com/auth/webmasters",
    ];
    for (const scope of forbidden) {
      expect(allScopes).not.toContain(scope);
    }
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
    expect(() => scopesForServices(["youtube"])).toThrow(UnknownServiceError);
    expect(() => normalizeServices(["not-a-service"])).toThrow(UnknownServiceError);
    expect(() => normalizeServices("analytics")).toThrow(UnknownServiceError);
    expect(isHostedService("analytics")).toBe(true);
    expect(isHostedService("youtube")).toBe(false);
  });

  it("normalizes and dedupes services case-insensitively", () => {
    expect(normalizeServices(["Analytics", " analytics ", "gmail"])).toEqual([
      "analytics",
      "gmail",
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
