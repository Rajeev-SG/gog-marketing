import { test } from "@playwright/test";
import { join } from "node:path";
import { LIMIT_ERROR, openChat, openHome } from "./audit-fixtures.js";

// Opt-in evidence capture for visual review (design-review / compare-screenshots).
// Run: CAPTURE_DIR=/abs/path pnpm exec playwright test --grep @visual
// Synthetic fixtures only: never point this at real accounts.
const dir = process.env.CAPTURE_DIR;
const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "mobile", width: 390, height: 844 },
];

for (const viewport of VIEWPORTS) {
  test.describe(`@visual ${viewport.name}`, () => {
    test.skip(!dir, "CAPTURE_DIR not set");
    test.use({ viewport: { width: viewport.width, height: viewport.height } });
    const shot = (name: string) => join(dir!, `${name}-${viewport.name}.png`);

    for (const [name, path] of [
      ["landing", "/"],
      ["about", "/about"],
      ["request-access", "/request-access"],
      ["not-found", "/settings"],
    ] as const) {
      test(`public ${name}`, async ({ page }) => {
        await page.goto(path);
        await page.screenshot({ path: shot(name), fullPage: true });
      });
    }

    for (const [name, path] of [
      ["home", "/"],
      ["data", "/data"],
      ["access", "/access"],
      ["connect-ai", "/connect-ai"],
    ] as const) {
      test(`workspace ${name}`, async ({ page }) => {
        await openHome(page, undefined, { path });
        await page.waitForTimeout(300);
        await page.screenshot({ path: shot(name), fullPage: true });
        await page.screenshot({ path: shot(name + "-viewport") });
      });
    }

    test("workspace unsaved and dialogs", async ({ page }) => {
      await openHome(page, undefined, { path: "/data" });
      await page.getByText("Google Analytics 4 asset 1").click();
      await page.screenshot({ path: shot("data-unsaved") });
      await page.getByRole("button", { name: /More actions for owner@example.com/ }).click();
      await page.screenshot({ path: shot("data-menu") });
      await page.getByRole("menuitem", { name: "Disconnect account" }).click();
      await page.screenshot({ path: shot("data-dialog") });
    });

    test("chat thread and failure", async ({ page }) => {
      await openChat(page, {
        failWith: LIMIT_ERROR,
        savedMessages: [
          {
            id: "u1",
            role: "user",
            parts: [{ type: "text", text: "Draft a spring launch brief" }],
          },
          {
            id: "a1",
            role: "assistant",
            parts: [
              {
                type: "text",
                text: "Here is a first draft.\n\n**Objective:** grow trial sign-ups by 20%.\n\n- Audience: lapsed users\n- Channels: email, paid social",
              },
            ],
          },
        ],
      });
      await page.screenshot({ path: shot("chat-thread") });
      await page.getByRole("textbox", { name: "Message", exact: true }).fill("Summarise results");
      await page.getByRole("button", { name: "Send message" }).click();
      await page.getByRole("button", { name: "Retry" }).waitFor();
      await page.screenshot({ path: shot("chat-failure") });
    });

    test("chat empty", async ({ page }) => {
      await openChat(page);
      await page.screenshot({ path: shot("chat-empty") });
    });
  });
}
