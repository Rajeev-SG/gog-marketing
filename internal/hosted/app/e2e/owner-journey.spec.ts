import { expect, test, type Page } from "@playwright/test";

const clerkEmail = process.env.TEST_CLERK_EMAIL;
const clerkPassword = process.env.TEST_CLERK_PASSWORD;

async function signInWithTestAccount(page: Page): Promise<void> {
  await page.goto("/");
  const email = page.locator('input[type="email"]').first();
  await expect(email).toBeVisible();
  await email.fill(clerkEmail!);
  await page
    .getByRole("button", { name: /continue|sign in|next/i })
    .first()
    .click();

  const password = page.locator('input[type="password"]').first();
  await expect(password).toBeVisible();
  await password.fill(clerkPassword!);
  await page
    .getByRole("button", { name: /continue|sign in|next/i })
    .first()
    .click();
}

test("@deterministic public home renders Clerk sign-in and API returns 401", async ({
  page,
  request,
}) => {
  const fixtureOrigin = new URL(process.env.PLAYWRIGHT_BASE_URL ?? "http://127.0.0.1:8788").origin;
  await page.route("**/*", async (route) => {
    const navigation = route.request();
    const url = new URL(navigation.url());
    if (url.origin !== fixtureOrigin) {
      await route.abort();
      return;
    }

    const upstream = await fetch(navigation.url(), {
      method: navigation.method(),
      headers: await navigation.allHeaders(),
      body: navigation.postDataBuffer() ?? undefined,
    });
    await route.fulfill({
      status: upstream.status,
      headers: Object.fromEntries(upstream.headers),
      body: Buffer.from(await upstream.arrayBuffer()),
    });
  });
  const response = await page.goto("/");

  expect(response?.status()).toBe(200);
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(page.locator("#sign-in")).toBeAttached();
  await expect(page.locator("script[data-clerk-publishable-key]").first()).toBeAttached();
  await expect(page.getByText("Clerk authentication")).toBeVisible();

  const api = await request.get("/api/tenant");
  expect(api.status()).toBe(401);
  await expect(api.json()).resolves.toMatchObject({
    error: expect.stringContaining("Authentication required"),
  });
});

test.describe("@owner Clerk sign-in, connect account, asset toggles, and MCP URL copy", () => {
  test.skip(
    !clerkEmail || !clerkPassword,
    "set TEST_CLERK_EMAIL and TEST_CLERK_PASSWORD for the real Clerk owner journey",
  );

  test("runs the real Clerk owner UI flow with mocked asset persistence", async ({
    page,
    context,
  }) => {
    let enabled = false;
    const savedGrantRequests: unknown[] = [];
    await page.route("**/api/google/resources", (route) =>
      route.fulfill({
        json: {
          connections: [
            {
              id: "connection-e2e",
              email: "owner@example.test",
              status: "active",
              services: ["analytics"],
              discovery: {
                status: "ok",
                detail: "ok",
                resourceCount: 1,
                checkedAt: "2026-10-08T00:00:00.000Z",
              },
              resources: [
                {
                  connectionId: "connection-e2e",
                  service: "analytics",
                  resourceId: "properties/123",
                  resourceType: "property",
                  displayName: "Example property",
                  enabled,
                },
              ],
            },
          ],
        },
      }),
    );
    await page.route("**/api/google/grants", async (route) => {
      const body = await route.request().json();
      savedGrantRequests.push(body);
      const grant = body.grants?.find(
        (item: { resourceId?: string }) => item.resourceId === "properties/123",
      );
      enabled = Boolean(grant?.enabled);
      await route.fulfill({ json: { ok: true } });
    });

    await signInWithTestAccount(page);

    await expect(page.getByRole("heading", { name: "Connect a Google account" })).toBeVisible();
    await expect(page.locator("#connect-google")).toBeVisible();
    await expect(page.locator("#asset-workspace")).toBeVisible();

    const asset = page.getByRole("checkbox", { name: "Example property access" });
    await expect(asset).not.toBeChecked();
    await asset.check();
    await page.getByRole("button", { name: "Save access" }).click();
    await expect(page.locator("#app-message")).toContainText("Access saved complete.");
    await expect(asset).toBeChecked();

    await asset.uncheck();
    await page.getByRole("button", { name: "Save access" }).click();
    await expect(page.locator("#app-message")).toContainText("Access saved complete.");
    await expect(asset).not.toBeChecked();

    expect(savedGrantRequests).toEqual([
      {
        connectionId: "connection-e2e",
        grants: [{ service: "analytics", resourceId: "properties/123", enabled: true }],
      },
      {
        connectionId: "connection-e2e",
        grants: [{ service: "analytics", resourceId: "properties/123", enabled: false }],
      },
    ]);

    const mcpURL = await page.locator("#mcp-url").innerText();
    expect(mcpURL).toMatch(/\/mcp$/);
    const copy = page.locator("#copy-mcp-url");
    await expect(copy).toBeEnabled();
    await context.grantPermissions(["clipboard-read", "clipboard-write"], {
      origin: new URL(page.url()).origin,
    });
    await copy.click();
    await expect(copy).toHaveText("Copied");
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(mcpURL);
  });
});

// This test exercises the built React/native-SDK client only. Identity and sockets
// are explicit fixtures; it does not claim real Clerk/OpenRouter acceptance.
test("@deterministic chat rechecks identity before recovering a terminal socket", async ({
  page,
}) => {
  const { readFileSync } = await import("node:fs");
  const { fileURLToPath } = await import("node:url");
  const { join } = await import("node:path");
  const dist = fileURLToPath(new URL("../dist/client/", import.meta.url));
  const conversation = "1".repeat(64);
  let authorized = true;
  let sessionChecks = 0;
  const sockets: import("@playwright/test").WebSocketRoute[] = [];
  const heldMetadata: import("@playwright/test").Route[] = [];
  let holdNextMetadata = false;
  await page.addInitScript(() => {
    const listeners = new Set<(value: { session: { id: string } | null }) => void>();
    const fixtureClerk = {
      loaded: !sessionStorage.getItem("fixture:delay-clerk"),
      session: { id: "fixture" },
      addListener: (fn: (value: { session: { id: string } | null }) => void) => {
        listeners.add(fn);
        fn({ session: fixtureClerk.session });
        return () => listeners.delete(fn);
      },
      signOut: async () => {},
    };
    Object.assign(window, {
      Clerk: fixtureClerk,
      __fixtureSession: (id: string) => {
        fixtureClerk.session = { id };
        for (const fn of listeners) fn({ session: fixtureClerk.session });
      },
      __fixtureListeners: () => listeners.size,
    });
  });
  await page.routeWebSocket("**/agents/**", (socket) => {
    sockets.push(socket);
  });
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/chat") {
      await route.fulfill({
        contentType: "text/html",
        body: readFileSync(join(dist, "chat.html")),
      });
    } else if (url.pathname.startsWith("/assets/")) {
      await route.fulfill({
        contentType: url.pathname.endsWith(".css") ? "text/css" : "application/javascript",
        body: readFileSync(join(dist, url.pathname.slice(1))),
      });
    } else if (url.pathname === "/api/chat/session") {
      sessionChecks++;
      if (holdNextMetadata) {
        holdNextMetadata = false;
        heldMetadata.push(route);
        return;
      }
      await route.fulfill({
        status: authorized ? 200 : 401,
        json: authorized
          ? {
              model: "openai/gpt-4.1-mini",
              conversation,
              draftKey: "fixture-reconnect",
              expiresAt: Date.now() + 3600000,
            }
          : { error: "Sign in to continue." },
      });
    } else if (url.pathname.endsWith("/get-messages")) {
      await route.fulfill({ json: [] });
    } else await route.abort();
  });
  await page.goto("/chat");
  const input = page.getByRole("textbox", { name: "Message" });
  await expect(input).toBeEnabled();
  await input.fill("draft retained through reconnect");
  await expect.poll(() => sockets.length).toBe(1);
  const before = sessionChecks;
  await sockets[0].close({ code: 4401, reason: "Fixture stale upgrade" });
  await expect.poll(() => sessionChecks).toBeGreaterThan(before);
  await expect.poll(() => sockets.length).toBe(2);
  expect(sessionChecks).toBeGreaterThan(before);
  await expect(input).toBeEnabled();
  await expect(input).toHaveValue("draft retained through reconnect");
  // The same terminal close must not reconnect if the authoritative API rejects identity.
  authorized = false;
  await sockets[1].close({ code: 4401, reason: "Fixture revoked session" });
  await expect(page.getByRole("link", { name: "Sign in to continue" })).toBeVisible();
  expect(sockets).toHaveLength(2);
  await expect
    .poll(() => page.evaluate(() => sessionStorage.getItem("gog:chat:draft:fixture-reconnect")))
    .toBe("draft retained through reconnect");
  // A stale owner-200 response must not remount its draft after Clerk changes identity.
  authorized = true;
  holdNextMetadata = true;
  const socketsBeforeSwitch = sockets.length;
  await page.reload();
  await expect.poll(() => heldMetadata.length).toBe(1);
  await expect.poll(() => page.evaluate(() => Reflect.get(window, "__fixtureListeners")())).toBe(1);
  authorized = false;
  await page.evaluate(() => Reflect.get(window, "__fixtureSession")("different-fixture-session"));
  await heldMetadata[0].fulfill({
    json: {
      model: "openai/gpt-4.1-mini",
      conversation,
      draftKey: "fixture-reconnect",
      expiresAt: Date.now() + 3600000,
    },
  });
  await expect(page.getByRole("link", { name: "Sign in to continue" })).toBeVisible();
  expect(sockets).toHaveLength(socketsBeforeSwitch);
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveCount(0);
  // Clerk arriving after the attach timeout still gets a fresh listener on Retry.
  authorized = true;
  await page.evaluate(() => sessionStorage.setItem("fixture:delay-clerk", "1"));
  await page.reload();
  await expect(page.getByRole("button", { name: "Retry connection" })).toBeVisible({
    timeout: 15000,
  });
  await page.evaluate(() => {
    Reflect.get(window, "Clerk").loaded = true;
    sessionStorage.removeItem("fixture:delay-clerk");
  });
  await page.getByRole("button", { name: "Retry connection" }).click();
  await expect(input).toBeEnabled();
  await expect.poll(() => page.evaluate(() => Reflect.get(window, "__fixtureListeners")())).toBe(1);
  authorized = false;
  await page.evaluate(() => Reflect.get(window, "__fixtureSession")("changed-after-retry"));
  await expect(page.getByRole("link", { name: "Sign in to continue" })).toBeVisible();
  await expect(input).toHaveCount(0);
});
