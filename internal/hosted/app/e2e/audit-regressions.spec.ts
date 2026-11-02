import { expect, test, type Locator } from "@playwright/test";
import {
  LIMIT_ERROR,
  axeViolations,
  horizontalOverflow,
  openChat,
  openHome,
} from "./audit-fixtures.js";

// Regression checks for the 31 Oct 2026 UX audit (FINDINGS.md) and design
// review (DESIGN-REVIEW.md). Each test names the finding IDs it guards.

/** React Aria checkboxes keep the native input visually hidden; users click its label. */
const toggle = (checkbox: Locator) => checkbox.locator("xpath=ancestor::label[1]").click();

const VIEWPORTS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "mobile", width: 390, height: 844 },
];

const emptyAssistantShell = [
  { id: "u1", role: "user", parts: [{ type: "text", text: "hello" }] },
  { id: "a1", role: "assistant", parts: [{ type: "step-start" }] },
];

test.describe("@deterministic chat failure turn (C01, C03, C05)", () => {
  test("a failed reply is one danger alert on the assistant turn with Retry", async ({ page }) => {
    await openChat(page, { failWith: LIMIT_ERROR });
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await input.fill("Summarise my campaign results");
    await expect(page.getByRole("button", { name: "Send message" })).toBeEnabled();
    await page.getByRole("button", { name: "Send message" }).click();
    const turn = page.locator(".message.assistant").last();
    await expect(turn.getByRole("alert")).toContainText(/usage limit/i);
    await expect(turn.getByRole("button", { name: /retry/i })).toBeVisible();
    // C03: the error is shown once, never in the neutral/success status pill.
    await expect(page.getByText(/usage limit/i)).toHaveCount(1);
    await expect(page.locator(".composer-status")).not.toContainText(/limit/i);
    // C02: the failure state must not widen the column on a phone.
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
  });

  test("persisted empty assistant shells are not shown as blank replies", async ({ page }) => {
    await openChat(page, { savedMessages: emptyAssistantShell });
    await expect(page.getByText("hello", { exact: true })).toBeVisible();
    const assistant = page.locator(".message.assistant");
    // Either hidden, or rendered as an explicit "no reply" state, never an empty bubble.
    const count = await assistant.count();
    for (let i = 0; i < count; i++) {
      await expect(assistant.nth(i)).toContainText(/\S/);
    }
  });

  test("usage and reset time are shown in plain language before sending", async ({ page }) => {
    await openChat(page);
    await page
      .getByText(/usage|limits/i)
      .first()
      .click();
    await expect(page.getByText(/used 100 of 100/i)).toBeVisible();
    await expect(page.getByText(/^Resets at /)).toBeVisible();
    await expect(page.getByText("openrouter/free")).toHaveCount(0);
  });
});

for (const viewport of VIEWPORTS) {
  test.describe(`@deterministic chat layout at ${viewport.name}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test("no horizontal overflow and Send is inside the viewport (C02)", async ({ page }) => {
      const { errors } = await openChat(page, {
        savedMessages: [
          { id: "u1", role: "user", parts: [{ type: "text", text: "Draft a brief ".repeat(40) }] },
          {
            id: "a1",
            role: "assistant",
            parts: [
              {
                type: "text",
                text: "A reply with a long URL https://example.com/" + "x".repeat(200),
              },
            ],
          },
        ],
      });
      await expect(page.getByRole("combobox", { name: /model/i })).toBeVisible();
      expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
      const send = await page.getByRole("button", { name: "Send message" }).boundingBox();
      expect(send).not.toBeNull();
      expect(send!.x + send!.width).toBeLessThanOrEqual(viewport.width);
      expect(errors).toEqual([]);
    });

    test("axe has no WCAG A/AA violations", async ({ page }) => {
      await openChat(page);
      expect(await axeViolations(page)).toEqual([]);
    });
  });

  test.describe(`@deterministic workspace at ${viewport.name}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    for (const path of ["/", "/data", "/access", "/connect-ai"]) {
      test(`${path} has no overflow, no page errors and is axe clean`, async ({ page }) => {
        const errors = await openHome(page, undefined, { path });
        await expect(page.locator("h1")).toHaveCount(1);
        expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
        expect(errors).toEqual([]);
        expect(await axeViolations(page)).toEqual([]);
      });
    }
  });
}

for (const viewport of VIEWPORTS) {
  test.describe(`@deterministic public pages at ${viewport.name} (P01, P02, DR12, L08)`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });
    for (const path of ["/", "/about", "/request-access", "/settings"]) {
      test(`${path} is axe clean, has one h1 and does not overflow`, async ({ page }) => {
        const response = await page.goto(path);
        if (path === "/settings") {
          expect(response?.status()).toBe(404);
          await expect(page.getByRole("link", { name: "Go to your workspace" })).toBeVisible();
        }
        await expect(page.locator("h1")).toHaveCount(1);
        await expect(page.locator('link[rel="icon"]')).toHaveCount(1);
        expect(
          await page
            .locator("h1, h2")
            .evaluateAll((els) => els.some((el) => el.querySelector("br"))),
        ).toBe(false);
        expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
        expect(await axeViolations(page)).toEqual([]);
      });
    }
  });
}

test.describe("@deterministic workspace routes (D05)", () => {
  test("old single-page anchors land on the matching page", async ({ page }) => {
    await openHome(page, undefined, { path: "/#google-data" });
    await expect(page).toHaveURL(/\/data$/);
    await expect(page.locator("h1")).toHaveText("Google data");
  });

  test("navigation is client-side and marks the current page", async ({ page }) => {
    await openHome(page);
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: "Access" })
      .click();
    await expect(page).toHaveURL(/\/access$/);
    await expect(page.locator("h1")).toHaveText("Access");
    await expect(
      page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Access" }),
    ).toHaveAttribute("aria-current", "page");
    await expect(page.locator("#main-content")).toBeFocused();
  });
});

test.describe("@deterministic home overview (D02, L01, L03)", () => {
  test("service rows count every granting account and open that service's assets", async ({
    page,
  }) => {
    await openHome(page);
    const analytics = page.locator(".service-row", { hasText: "Google Analytics" });
    // Both fixture accounts granted Analytics; only one has discovered assets.
    await expect(analytics).toContainText("2 accounts");
    await expect(analytics).toContainText("2 of 2");
    await expect(page.locator(".service-row", { hasText: "Google Tag Manager" })).toContainText(
      "1 account",
    );
    await page.locator(".service-row", { hasText: "Google Tag Manager" }).click();
    await expect(page).toHaveURL(/\/data\?service=tagmanager/);
    await expect(page.getByRole("tab", { name: /Google Tag Manager/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(page.locator(".service-mark").first()).toHaveAttribute("aria-hidden", "true");
  });

  test("the setup banner shows only while setup is incomplete", async ({ page }) => {
    await openHome(page);
    await expect(page.getByRole("heading", { name: "Finish setting up" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Choose AI tools" })).toBeVisible();
  });

  test("the setup banner is hidden once every step is complete", async ({ page }) => {
    await openHome(page, undefined, { toolsEnabled: true });
    await expect(page.locator(".metric-strip")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Finish setting up" })).toHaveCount(0);
  });
});

test.describe("@deterministic Google data account context (U03, D01, N04)", () => {
  test("switching to an empty account resets the summary and names the account", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/data" });
    await expect(page.locator(".asset-summary")).toContainText("3 selected");
    await page
      .locator(".account-card", { hasText: "empty@example.com" })
      .getByRole("button", { name: "Manage data access" })
      .click();
    await expect(page.getByText("No assets found yet")).toBeVisible();
    await expect(page.locator(".asset-summary")).toContainText("0 selected");
    await expect(page.locator(".asset-summary")).not.toContainText("All services healthy");
    await expect(page.locator("#assets")).toContainText("empty@example.com");
    await expect(page.locator(".account-card", { hasText: "empty@example.com" })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  test("the account menu names its account, closes on Escape and returns focus", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/data" });
    const trigger = page.getByRole("button", { name: "More actions for owner@example.com" });
    await trigger.click();
    await expect(page.getByRole("menu")).toContainText("Actions for owner@example.com");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test("disconnect asks first in an accessible dialog; Cancel changes nothing", async ({
    page,
  }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { path: "/data", posts });
    const open = async () => {
      await page.getByRole("button", { name: "More actions for owner@example.com" }).click();
      await page.getByRole("menuitem", { name: "Disconnect account" }).click();
    };
    await open();
    const dialog = page.getByRole("alertdialog", { name: "Disconnect owner@example.com?" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    expect(posts).toEqual([]);
    await open();
    await page.getByRole("button", { name: "Disconnect account" }).click();
    await expect.poll(() => posts).toEqual(["/api/google/connections/conn-populated/disconnect"]);
  });

  test("Update Google permissions opens the targeted form without starting OAuth", async ({
    page,
  }) => {
    const connectBodies: unknown[] = [];
    await openHome(page, undefined, { path: "/data", connectBodies });
    await page.getByRole("button", { name: "More actions for owner@example.com" }).click();
    await page.getByRole("menuitem", { name: "Update Google permissions" }).click();
    await expect(page).toHaveURL(/\/access\?update=conn-populated#connect$/);
    await expect(
      page.getByRole("heading", { name: "Update Google permissions for owner@example.com" }),
    ).toBeVisible();
    expect(connectBodies).toEqual([]);
  });
});

test.describe("@deterministic connect form consent rules", () => {
  test("writes need reads; select all and clear; the submit sends exactly the choices", async ({
    page,
  }) => {
    const connectBodies: unknown[] = [];
    await openHome(page, undefined, { path: "/access?update=conn-populated", connectBodies });
    const writes = page.getByRole("checkbox", { name: "Writes for Google Docs" });
    await expect(writes).toBeDisabled();
    await toggle(page.getByRole("checkbox", { name: "Google Docs", exact: true }));
    await expect(writes).toBeEnabled();
    await toggle(writes);
    await toggle(page.getByRole("checkbox", { name: "Google Docs", exact: true }));
    await expect(writes).not.toBeChecked();
    await expect(writes).toBeDisabled();
    await page.getByRole("button", { name: "Select all services" }).click();
    await expect(page.getByRole("checkbox", { name: "BigQuery", exact: true })).toBeChecked();
    await page.getByRole("button", { name: "Clear selection" }).click();
    await expect(page.getByRole("checkbox", { name: "BigQuery", exact: true })).not.toBeChecked();
    await toggle(page.getByRole("checkbox", { name: "Google Sheets", exact: true }));
    await toggle(page.getByRole("checkbox", { name: "Writes for Google Sheets" }));
    await page.getByRole("button", { name: "Update Google permissions" }).click();
    await expect
      .poll(() => connectBodies)
      .toEqual([
        { services: ["sheets"], writeServices: ["sheets"], connectionId: "conn-populated" },
      ]);
  });
});

test.describe("@deterministic asset picker (DR09, L06)", () => {
  test("Cancel discards, search shows filtered counts, select-all is tri-state", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/data" });
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toHaveCount(0);
    const first = page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" });
    await toggle(first);
    const bar = page.getByRole("region", { name: "Unsaved changes" });
    await expect(bar).toContainText("2 assets selected");
    // Native indeterminate state, announced as "mixed" by screen readers.
    expect(
      await page
        .getByRole("checkbox", { name: "All Google Analytics 4 assets" })
        .evaluate((input) => (input as HTMLInputElement).indeterminate),
    ).toBe(true);
    await bar.getByRole("button", { name: "Cancel" }).click();
    await expect(first).toBeChecked();
    await expect(bar).toHaveCount(0);
    await page.getByRole("searchbox", { name: "Search assets" }).fill("asset 2");
    await expect(page.locator(".service-meta")).toHaveText(/Showing 1 of 2 assets matching/);
  });
});

test.describe("@deterministic tool permissions (#107)", () => {
  test("enabling a tool that changes data needs confirmation; Cancel leaves it off", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/data" });
    await page.getByRole("button", { name: /Google Tag Manager \(Marketing\)/ }).click();
    const tool = page.getByRole("checkbox", { name: /^Google Tag Manager · Update Tag/ });
    await toggle(tool);
    const dialog = page.getByRole("alertdialog", {
      name: "Allow this tool to change Google data?",
    });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(tool).not.toBeChecked();
    await expect(page.getByRole("checkbox", { name: /Also allow deleting/ })).toBeDisabled();
    await toggle(tool);
    await page.getByRole("button", { name: "Allow changes" }).click();
    await expect(tool).toBeChecked();
    await expect(page.getByRole("checkbox", { name: /Also allow deleting/ })).toBeEnabled();
  });

  test("the bulk read action never enables a write tool", async ({ page }) => {
    await openHome(page, undefined, { path: "/data" });
    await page.getByRole("button", { name: "Enable read-only tools in this list" }).click();
    await expect(page.locator(".tool-summary")).toContainText("1 enabled");
    await page.getByRole("button", { name: /Google Tag Manager \(Marketing\)/ }).click();
    await expect(
      page.getByRole("checkbox", { name: /^Google Tag Manager · Update Tag/ }),
    ).not.toBeChecked();
  });
});

test.describe("@deterministic one save model (D03 decision A)", () => {
  test("assets and tools share one save bar and save with one click", async ({ page }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { path: "/data", posts });
    await toggle(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }));
    await page.getByRole("button", { name: /Google Analytics 4 \(Marketing\)/ }).click();
    await toggle(page.getByRole("checkbox", { name: /^Google Analytics 4 · Run Report/ }));
    const bar = page.getByRole("region", { name: "Unsaved changes" });
    await expect(bar).toContainText("Unsaved: 1 asset change and tool permission changes");
    await expect(page.getByRole("button", { name: "Save changes" })).toHaveCount(1);
    await bar.getByRole("button", { name: "Save changes" }).click();
    await expect.poll(() => posts).toEqual(["/api/google/tools", "/api/google/grants"]);
  });

  test("a failed tool save stops before the asset save so no draft is lost", async ({ page }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { path: "/data", posts, failToolSave: true });
    await toggle(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }));
    await page.getByRole("button", { name: /Google Analytics 4 \(Marketing\)/ }).click();
    await toggle(page.getByRole("checkbox", { name: /^Google Analytics 4 · Run Report/ }));
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByRole("alert").first()).toContainText("temporarily unavailable");
    expect(posts).toEqual(["/api/google/tools"]);
    await expect(
      page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }),
    ).not.toBeChecked();
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toContainText(
      "1 asset change",
    );
  });

  test("Cancel discards both asset and tool drafts", async ({ page }) => {
    await openHome(page, undefined, { path: "/data" });
    await toggle(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }));
    await page.getByRole("button", { name: /Google Analytics 4 \(Marketing\)/ }).click();
    await toggle(page.getByRole("checkbox", { name: /^Google Analytics 4 · Run Report/ }));
    await page
      .getByRole("region", { name: "Unsaved changes" })
      .getByRole("button", { name: "Cancel" })
      .click();
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toHaveCount(0);
    await expect(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" })).toBeChecked();
  });

  test("switching account with unsaved tool changes asks before discarding", async ({ page }) => {
    await openHome(page, undefined, { path: "/data" });
    await page.getByRole("button", { name: /Google Analytics 4 \(Marketing\)/ }).click();
    await toggle(page.getByRole("checkbox", { name: /^Google Analytics 4 · Run Report/ }));
    await page
      .locator(".account-card", { hasText: "empty@example.com" })
      .getByRole("button", { name: "Manage data access" })
      .click();
    const dialog = page.getByRole("alertdialog", { name: "Discard unsaved changes?" });
    await dialog.getByRole("button", { name: "Keep editing" }).click();
    await expect(page.locator(".account-card", { hasText: "owner@example.com" })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });
});
