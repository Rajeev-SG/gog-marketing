import { expect, test, type Route } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

// Actual built UI with explicit identity/network fixtures, not live Clerk/provider proof.
for (const width of [1280, 390]) {
  test(`@deterministic account switching isolates sidebar, history and drafts at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 844 });
    const dist = fileURLToPath(new URL("../dist/client/", import.meta.url));
    let identity = "A";
    let heldList: Route | undefined;
    let holdBList = true;
    const digest = (user: string) => (user === "A" ? "a" : "b").repeat(64);
    const thread = (user: string) => ({
      id: "original",
      title: `${user} private thread`,
      createdAt: "2026-10-18T00:00:00Z",
      updatedAt: "2026-10-18T00:00:00Z",
    });
    await page.addInitScript(() => {
      const listeners = new Set<(value: { session: { id: string } }) => void>();
      const fixture = {
        loaded: true,
        session: { id: "A" },
        addListener(fn: (value: { session: { id: string } }) => void) {
          listeners.add(fn);
          fn({ session: fixture.session });
          return () => listeners.delete(fn);
        },
        async signOut() {},
      };
      Object.assign(window, {
        Clerk: fixture,
        __switchUser: (id: string) => {
          fixture.session = { id };
          for (const fn of listeners) fn({ session: fixture.session });
        },
        __listenerCount: () => listeners.size,
      });
    });
    await page.routeWebSocket("**/agents/**", () => {});
    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === "/chat")
        return route.fulfill({
          contentType: "text/html",
          body: readFileSync(join(dist, "chat.html")),
        });
      if (url.pathname.startsWith("/assets/"))
        return route.fulfill({
          contentType: url.pathname.endsWith(".css") ? "text/css" : "application/javascript",
          body: readFileSync(join(dist, url.pathname.slice(1))),
        });
      if (url.pathname === "/api/chat/session")
        return route.fulfill({
          json: {
            thread: thread(identity),
            model: "openai/gpt-4.1-mini",
            conversation: digest(identity),
            draftKey: digest(identity),
            expiresAt: Date.now() + 3600000,
          },
        });
      if (url.pathname === "/api/chat/threads") {
        if (identity === "B" && holdBList) {
          heldList = route;
          return;
        }
        return route.fulfill({ json: { threads: [thread(identity)], pendingDeletions: [] } });
      }
      if (url.pathname.endsWith("/get-messages")) {
        const user = url.pathname.includes(digest("A")) ? "A" : "B";
        return route.fulfill({
          json: [
            {
              id: `${user}-answer`,
              role: "assistant",
              parts: [{ type: "text", text: `${user} private answer` }],
            },
          ],
        });
      }
      return route.abort();
    });
    await page.goto("/chat?thread=original");
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await expect(page.getByText("A private answer", { exact: true })).toBeVisible();
    await expect(page.locator('a[title="A private thread"]')).toHaveCount(1);
    await input.fill("A unsent private draft");
    await expect.poll(() => page.evaluate(() => Reflect.get(window, "__listenerCount")())).toBe(1);
    identity = "B";
    await page.evaluate(() => Reflect.get(window, "__switchUser")("B"));
    await expect(
      page.getByRole("heading", { name: "B private thread", exact: true }),
    ).toBeVisible();
    await expect.poll(() => Boolean(heldList)).toBe(true);
    // B is mounted while its sidebar fetch is pending: A's metadata must already be gone.
    await expect(page.locator('a[title="A private thread"]')).toHaveCount(0);
    await expect(page.getByText("A private answer", { exact: true })).toHaveCount(0);
    await expect(page.getByText("B private answer", { exact: true })).toBeVisible();
    await expect(input).toHaveValue("");
    holdBList = false;
    await heldList!.fulfill({ json: { threads: [thread("B")], pendingDeletions: [] } });
    await expect(page.locator('a[title="B private thread"]')).toHaveCount(1);
    await input.fill("B unsent private draft");
    await page.screenshot({ path: test.info().outputPath("isolated-user-b.png"), fullPage: true });
    identity = "A";
    await page.evaluate(() => Reflect.get(window, "__switchUser")("A"));
    await expect(
      page.getByRole("heading", { name: "A private thread", exact: true }),
    ).toBeVisible();
    await expect(input).toHaveValue("A unsent private draft");
    await expect(page.getByText("B private answer", { exact: true })).toHaveCount(0);
    await expect(page.locator('a[title="B private thread"]')).toHaveCount(0);
    await expect
      .poll(() => page.evaluate(() => sessionStorage.getItem("gog:chat:draft:" + "b".repeat(64))))
      .toBe("B unsent private draft");
  });
}
