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
