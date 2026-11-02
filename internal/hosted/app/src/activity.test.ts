import { describe, expect, it } from "vitest";
import { describeAuditEvent } from "./activity.js";

const event = (action: string, result: string, detail: Record<string, unknown>) => ({
  id: "e1",
  tenantId: "t1",
  connectionId: "c1",
  actorClerkUserId: "user_secret",
  action,
  result: result as "allow" | "deny" | "error" | "info",
  detailJson: JSON.stringify(detail),
  latencyMs: 12,
  createdAt: "2026-10-31T22:42:17.958Z",
});

describe("activity log (N01)", () => {
  it("describes AI tool calls in plain language, with the reason for a block", () => {
    const ran = describeAuditEvent(
      event("mcp.tools.call", "allow", {
        operation: "analytics_properties_get",
        service: "analytics",
        resource: "properties/498363924",
      }),
    );
    expect(ran).toMatchObject({
      kind: "tool",
      tone: "ok",
      title: "Google Analytics 4: Properties get ran",
      detail: "On properties/498363924",
    });
    const blocked = describeAuditEvent(
      event("mcp.tools.call", "deny", {
        operation: "analytics_properties_get",
        service: "analytics",
        error: "permission_denied",
      }),
    );
    expect(blocked.tone).toBe("blocked");
    expect(blocked.detail).toBe(
      "Blocked because the tool or asset isn’t enabled for this account.",
    );
  });

  it("describes setting changes and never leaks actor IDs or unknown detail", () => {
    const saved = describeAuditEvent(
      event("google.grants.save", "allow", { count: 3, token: "secret", operation: "save" }),
    );
    expect(saved).toEqual({
      id: "e1",
      at: "2026-10-31T22:42:17.958Z",
      connectionId: "c1",
      kind: "setting",
      tone: "info",
      title: "Data access saved",
      detail: "3 assets changed",
    });
    expect(JSON.stringify(saved)).not.toMatch(/secret|user_secret/);
    expect(describeAuditEvent(event("something.new", "info", {})).title).toBe("Workspace activity");
    expect(
      describeAuditEvent({ ...event("google.tools.save", "allow", {}), detailJson: "{" }),
    ).toMatchObject({ title: "AI tool permissions saved", detail: "" });
  });
});
