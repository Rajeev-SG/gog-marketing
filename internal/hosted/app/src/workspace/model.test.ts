import { afterEach, describe, expect, it, vi } from "vitest";

import {
  attentionBySection,
  attentionItems,
  changedGrants,
  hiddenToolCount,
  recentBlockedCount,
  toolSummary,
  visibleToolGroups,
  type Tool,
  connectOutcome,
  pendingSummary,
  requestConnection,
  resourceKey,
  safeStatus,
  serviceOverview,
  setupStep,
  workspaceSummary,
  type Connection,
} from "./model.js";

const resource = (service: string, id: string, enabled: boolean) => ({
  service,
  resourceId: id,
  displayName: `${service} ${id}`,
  enabled,
});
const populated: Connection = {
  id: "conn-populated",
  email: "owner@example.com",
  status: "active",
  discovery: { status: "complete" },
  services: ["Google Analytics 4", "Google Tag Manager", "Drive (selected files)"],
  resources: [
    resource("analytics", "1", true),
    resource("analytics", "2", true),
    resource("tagmanager", "3", false),
  ],
};
const empty: Connection = {
  id: "conn-empty",
  email: "empty@example.com",
  status: "active",
  discovery: { status: "empty" },
  services: ["Google Analytics 4"],
  resources: [],
};

describe("workspace summary and service overview (D02, L03)", () => {
  it("counts accounts, enabled assets and every granted service", () => {
    const summary = workspaceSummary([populated, empty]);
    expect(summary).toMatchObject({
      accounts: 2,
      enabledAssets: 2,
      totalAssets: 3,
      health: "All healthy",
    });
    expect(summary.services).toBe(3);
  });

  it("counts every account that granted a service, even without discovered assets", () => {
    const rows = serviceOverview([populated, empty]);
    const analytics = rows.find((row) => row.service === "analytics")!;
    expect(analytics).toMatchObject({
      accounts: 2,
      total: 2,
      enabled: 2,
      status: "ok",
      ownerId: "conn-populated",
    });
    expect(rows.find((row) => row.service === "tagmanager")).toMatchObject({
      accounts: 1,
      enabled: 0,
    });
    expect(rows.find((row) => row.service === "drive")).toMatchObject({
      total: 0,
      status: "empty",
    });
  });

  it("flags attention before delay, and only shows status when abnormal", () => {
    expect(safeStatus({ ...populated, status: "needs_reconnect" })).toEqual([
      "needs_attention",
      "Needs attention",
    ]);
    expect(safeStatus({ ...populated, discovery: { status: "error" } })).toEqual([
      "unavailable",
      "Check delayed",
    ]);
    expect(safeStatus(empty)).toEqual(["disconnected", "Ready"]);
    expect(workspaceSummary([{ ...populated, status: "needs_reconnect" }]).health).toBe(
      "Needs attention",
    );
  });
});

describe("setup guide (L01)", () => {
  it("walks connect → data → tools and disappears once complete", () => {
    expect(setupStep(undefined, false).action).toEqual({
      label: "Connect Google account",
      href: "/settings/accounts#connect",
    });
    expect(setupStep(empty, false).action.label).toBe("Choose data");
    expect(setupStep(populated, false).action).toEqual({
      label: "Choose AI tools",
      href: "/settings/tools",
    });
    expect(setupStep(populated, true).complete).toBe(true);
  });
});

describe("one save model (D03)", () => {
  it("reports only real differences from what is saved", () => {
    const draft = new Map([
      [resourceKey(populated.resources![0]!), false],
      [resourceKey(populated.resources![1]!), true],
    ]);
    expect(changedGrants(populated, draft)).toEqual([
      { service: "analytics", resourceId: "1", enabled: false },
    ]);
    expect(pendingSummary(1, true)).toBe("Unsaved: 1 asset change and tool permission changes");
    expect(pendingSummary(0, false)).toBe("No unsaved changes");
  });
});

describe("Google connection start", () => {
  afterEach(() => vi.useRealTimers());
  const body = { services: ["drive"], writeServices: [] };
  const response = (ok: boolean, json: () => Promise<unknown>) =>
    ({ ok, json }) as unknown as Response;

  it("returns the Google authorization URL and sends exactly the chosen services", async () => {
    const fetcher = vi.fn(async () =>
      response(true, async () => ({ authorizationUrl: "https://accounts.google.com/x" })),
    );
    await expect(
      requestConnection({ ...body, connectionId: "c1" }, fetcher as typeof fetch),
    ).resolves.toBe("https://accounts.google.com/x");
    const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/google/connect");
    expect(JSON.parse(String(init.body))).toEqual({
      services: ["drive"],
      writeServices: [],
      connectionId: "c1",
    });
  });

  it("classifies a stalled request as a timeout with retry guidance", async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn(
      (_url: string, init: RequestInit) =>
        new Promise<Response>((_, reject) =>
          init.signal!.addEventListener("abort", () => reject(init.signal!.reason)),
        ),
    );
    const pending = requestConnection(body, fetcher as unknown as typeof fetch, 20_000);
    const assertion = expect(pending).rejects.toThrow(
      "The Google sign-in request timed out. Check your network, then select the button again.",
    );
    await vi.advanceTimersByTimeAsync(20_000);
    await assertion;
  });

  it("treats an abort while reading the body as a timeout too", async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn(async (_url: string, init: RequestInit) =>
      response(
        true,
        () =>
          new Promise((_, reject) =>
            init.signal!.addEventListener("abort", () => reject(init.signal!.reason)),
          ),
      ),
    );
    const pending = requestConnection(body, fetcher as unknown as typeof fetch, 1000);
    const assertion = expect(pending).rejects.toThrow("timed out");
    await vi.advanceTimersByTimeAsync(1000);
    await assertion;
  });

  it("appends retry guidance to network, malformed and server errors", async () => {
    const network = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    await expect(requestConnection(body, network as typeof fetch)).rejects.toThrow(
      "Failed to fetch. Check your network, then select the button again.",
    );
    const malformed = vi.fn(async () =>
      response(false, () => Promise.reject(new SyntaxError("Unexpected token <"))),
    );
    await expect(requestConnection(body, malformed as typeof fetch)).rejects.toThrow(
      "Could not start Google connection. Check your network, then select the button again.",
    );
    const server = vi.fn(async () =>
      response(false, async () => ({ error: "Invalid service selection." })),
    );
    await expect(requestConnection(body, server as typeof fetch)).rejects.toThrow(
      "Invalid service selection. Check your network, then select the button again.",
    );
  });

  it("maps OAuth outcomes to plain messages, with a safe fallback", () => {
    expect(connectOutcome("identity_conflict")[0]).toBe(
      "Already connected — this Google account is already available.",
    );
    expect(connectOutcome("reconnect_identity_mismatch")[1]).toBe("error");
    expect(connectOutcome("made_up")[0]).toBe(
      "The Google connection attempt could not be completed.",
    );
  });
});

describe("settings attention (IA refresh)", () => {
  const tool = (name: string, service: string, over: Partial<Tool> = {}): Tool => ({
    name,
    service,
    description: "",
    write: false,
    enabled: false,
    allowDestructive: false,
    ...over,
  });
  const catalog = [
    tool("analytics_properties_get", "analytics", { enabled: true }),
    tool("analytics_properties_update", "analytics", {
      write: true,
      enabled: true,
      allowDestructive: true,
    }),
    tool("youtube_channels_list", "youtube", { enabled: true }),
    tool("adsense_accounts_list", "adsense"),
  ];

  it("summarises tools by what the account shares, counting stranded grants", () => {
    expect(toolSummary(populated, catalog)).toEqual({
      available: 2,
      enabled: 2,
      changes: 1,
      destructive: 1,
      stranded: 1,
    });
    expect(hiddenToolCount(populated, catalog)).toEqual({ tools: 2, services: 2 });
  });

  it("hides unshared services by default but keeps groups with stranded grants", () => {
    const choices = new Map(catalog.map((t) => [t.name, { toolName: t.name, ...t }]));
    expect(visibleToolGroups(populated, catalog, choices, false).map((g) => g.service)).toEqual([
      "analytics",
      "youtube",
    ]);
    expect(visibleToolGroups(populated, catalog, choices, true)).toHaveLength(3);
  });

  it("lists only what needs the user, most urgent first", () => {
    const broken = { ...empty, id: "conn-broken", status: "needs_reconnect" };
    const unchosen: Connection = {
      ...populated,
      id: "conn-unchosen",
      resources: [resource("analytics", "9", false)],
    };
    const items = attentionItems(
      [populated, broken, unchosen],
      new Map([
        ["conn-populated", catalog],
        ["conn-unchosen", []],
      ]),
      2,
    );
    expect(items.map((item) => item.id)).toEqual([
      "reconnect:conn-broken",
      "stranded:conn-populated",
      "data:conn-unchosen",
      "blocked",
    ]);
    expect(items[0]!.action.href).toBe("/settings/accounts?update=conn-broken#connect");
    expect(attentionBySection(items)).toEqual({ accounts: 1, tools: 1, data: 1, activity: 1 });
  });

  it("says nothing when everything is healthy, and prompts a first connection", () => {
    const healthy = [tool("analytics_properties_get", "analytics", { enabled: true })];
    expect(attentionItems([populated], new Map([["conn-populated", healthy]]))).toEqual([]);
    expect(attentionItems([], new Map()).map((item) => item.id)).toEqual(["connect"]);
  });

  it("counts blocked and failed AI requests from the last 7 days only", () => {
    const now = Date.parse("2026-11-02T12:00:00Z");
    const event = (at: string, tone: "ok" | "blocked" | "failed") => ({
      id: at,
      at,
      connectionId: "",
      kind: "tool" as const,
      tone,
      title: "",
      detail: "",
    });
    expect(
      recentBlockedCount(
        [
          event("2026-11-01T00:00:00Z", "blocked"),
          event("2026-10-30T00:00:00Z", "failed"),
          event("2026-11-01T00:00:00Z", "ok"),
          event("2026-10-20T00:00:00Z", "blocked"),
        ],
        now,
      ),
    ).toBe(2);
  });
});
