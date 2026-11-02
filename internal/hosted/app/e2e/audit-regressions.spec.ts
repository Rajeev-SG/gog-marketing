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

const WORKSPACE_PAGES = [
  "/",
  "/settings",
  "/settings/accounts",
  "/settings/data",
  "/settings/tools",
  "/settings/activity",
  "/settings/connect-ai",
];

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

    for (const path of WORKSPACE_PAGES) {
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
    for (const path of ["/", "/about", "/request-access", "/no-such-page"]) {
      test(`${path} is axe clean, has one h1 and does not overflow`, async ({ page }) => {
        const response = await page.goto(path);
        if (path === "/no-such-page") {
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

test.describe("@deterministic workspace routes (D05, IA refresh)", () => {
  test("old single-page anchors land on the matching page", async ({ page }) => {
    await openHome(page, undefined, { path: "/#google-data" });
    await expect(page).toHaveURL(/\/settings\/data$/);
    await expect(page.locator("h1")).toHaveText("Data access");
  });

  test("the old /data#tools link (now /settings/data#tools) opens AI tools", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings/data#tools" });
    await expect(page).toHaveURL(/\/settings\/tools$/);
    await expect(page.locator("h1")).toHaveText("AI tools");
  });

  test("the sidebar has three destinations and Settings shows what needs attention", async ({
    page,
  }) => {
    await openHome(page);
    const primary = page.getByRole("navigation", { name: "Primary" });
    await expect(primary.getByRole("link")).toHaveText([/Home/, /Chat/, /Settings/]);
    // Fixture: tools are off for an account with data, and one AI request was blocked.
    await expect(primary.getByRole("link", { name: /Settings/ })).toContainText("2");
    await primary.getByRole("link", { name: /Settings/ }).click();
    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.locator("h1")).toHaveText("Settings");
    await expect(primary.getByRole("link", { name: /Settings/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect(page.locator("#main-content")).toBeFocused();
    const sections = page.getByRole("navigation", { name: "Settings sections" });
    await sections.getByRole("link", { name: /AI tools/ }).click();
    await expect(page).toHaveURL(/\/settings\/tools$/);
    await expect(page.locator("h1")).toHaveText("AI tools");
  });
});

test.describe("@deterministic settings overview (IA refresh)", () => {
  test("lists what needs attention, most urgent first, each with a fix", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings" });
    const panel = page.locator(".attention-panel");
    await expect(panel.getByRole("heading", { level: 2 })).toHaveText(
      "2 things need your attention",
    );
    await expect(panel.locator(".attention-item")).toHaveText([
      /Choose AI tools for owner@example.com/,
      /1 AI request blocked or failed in the last 7 days/,
    ]);
    await panel.getByRole("link", { name: "Choose AI tools" }).click();
    await expect(page).toHaveURL(/\/settings\/tools\?account=conn-populated$/);
    // The same item is repeated where it can be fixed.
    await expect(page.locator(".attention-panel")).toContainText("Choose AI tools");
    const sections = page.getByRole("navigation", { name: "Settings sections" });
    await expect(sections.getByRole("link", { name: /AI tools/ })).toContainText("needs attention");
    await expect(sections.getByRole("link", { name: /Data access/ })).not.toContainText(
      "needs attention",
    );
  });

  test("says so plainly when nothing needs attention", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings", toolsEnabled: true, activity: [] });
    await expect(page.getByRole("heading", { name: "Nothing needs your attention" })).toBeVisible();
    await expect(
      page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: /Settings/ }),
    ).toHaveText("Settings");
  });

  test("Home shows attention items the setup guide doesn't already cover", async ({ page }) => {
    await openHome(page);
    // "Choose AI tools" is the setup guide's next step, so it isn't repeated.
    await expect(page.getByRole("heading", { name: "Finish setting up" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "1 thing needs your attention" })).toBeVisible();
    await expect(page.locator(".metric-strip")).toContainText("2 to review");
    await expect(page.getByRole("heading", { name: "How access works" })).toHaveCount(0);
  });
});

test.describe("@deterministic activity log (N01)", () => {
  test("shows real events by day and filters to problems", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings/activity" });
    await expect(page.locator(".activity-item")).toHaveCount(2);
    await expect(page.locator(".activity-item").first()).toContainText("owner@example.com");
    await page.getByRole("radio", { name: "Problems" }).click();
    await expect(page).toHaveURL(/show=problems$/);
    await expect(page.locator(".activity-item")).toHaveCount(1);
    await expect(page.locator(".activity-item")).toContainText("was blocked");
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
    await expect(page).toHaveURL(/\/settings\/data\?service=tagmanager/);
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

/** Choose an account in the Data access / AI tools account picker. */
async function chooseAccount(page: import("@playwright/test").Page, email: string) {
  await page.getByRole("button", { name: /Google account/ }).click();
  await page.getByRole("option", { name: new RegExp(email) }).click();
}

test.describe("@deterministic account context (U03, D01, N04)", () => {
  test("switching to an empty account resets the summary and names the account", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/settings/data" });
    await expect(page.locator(".asset-summary")).toContainText("3 of 4 enabled");
    await chooseAccount(page, "empty@example.com");
    await expect(page).toHaveURL(/account=conn-empty/);
    await expect(page.getByText("No assets found yet")).toBeVisible();
    await expect(page.locator(".asset-summary")).toContainText("0 of 0 enabled");
    await expect(page.locator("#assets")).toContainText("empty@example.com");
  });

  test("the account menu names its account, closes on Escape and returns focus", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/settings/accounts" });
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
    await openHome(page, undefined, { path: "/settings/accounts", posts });
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

  test("Change shared services opens the targeted form without starting OAuth", async ({
    page,
  }) => {
    const connectBodies: unknown[] = [];
    await openHome(page, undefined, { path: "/settings/accounts", connectBodies });
    await page.getByRole("button", { name: "More actions for owner@example.com" }).click();
    await page.getByRole("menuitem", { name: "Change shared services" }).click();
    await expect(page).toHaveURL(/\/settings\/accounts\?update=conn-populated#connect$/);
    await expect(
      page.getByRole("heading", { name: "Change shared services for owner@example.com" }),
    ).toBeVisible();
    expect(connectBodies).toEqual([]);
  });

  test("each account card links straight to its data and tools", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings/accounts" });
    const card = page.locator(".account-card", { hasText: "empty@example.com" });
    await expect(card.getByRole("link", { name: "Data" })).toHaveAttribute(
      "href",
      "/settings/data?account=conn-empty",
    );
    await expect(card.getByRole("link", { name: "AI tools" })).toHaveAttribute(
      "href",
      "/settings/tools?account=conn-empty",
    );
  });
});

test.describe("@deterministic connect form consent rules", () => {
  test("writes need reads; select all and clear; the submit sends exactly the choices", async ({
    page,
  }) => {
    const connectBodies: unknown[] = [];
    await openHome(page, undefined, {
      path: "/settings/accounts?update=conn-populated",
      connectBodies,
    });
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
    await page.getByRole("button", { name: "Continue to Google" }).click();
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
    await openHome(page, undefined, { path: "/settings/data" });
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
    await openHome(page, undefined, { path: "/settings/tools" });
    await page.getByRole("button", { name: /Google Tag Manager/ }).click();
    const tool = page.getByRole("checkbox", { name: /^Update Tag/ });
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
    await openHome(page, undefined, { path: "/settings/tools" });
    await page.getByRole("button", { name: "Turn on all read-only tools shown" }).click();
    await expect(page.locator(".tool-summary")).toContainText("1 enabled");
    await page.getByRole("button", { name: /Google Tag Manager/ }).click();
    await expect(page.getByRole("checkbox", { name: /^Update Tag/ })).not.toBeChecked();
  });
});

/** Move between Settings sections client-side, as a user would. */
async function openSection(page: import("@playwright/test").Page, name: string) {
  await page
    .getByRole("navigation", { name: "Settings sections" })
    .getByRole("link", { name: new RegExp(name) })
    .click();
  await expect(page.locator("h1")).toHaveText(name === "Overview" ? "Settings" : name);
}
async function draftAssetAndTool(page: import("@playwright/test").Page) {
  await toggle(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }));
  await openSection(page, "AI tools");
  await page.getByRole("button", { name: /Google Analytics 4/ }).click();
  await toggle(page.getByRole("checkbox", { name: /^Run Report/ }));
}

test.describe("@deterministic one save model (D03 decision A)", () => {
  test("asset and tool drafts survive moving between pages and save with one click", async ({
    page,
  }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { path: "/settings/data", posts });
    await draftAssetAndTool(page);
    const bar = page.getByRole("region", { name: "Unsaved changes" });
    await expect(bar).toContainText("Unsaved: 1 asset change and tool permission changes");
    await openSection(page, "Overview");
    await expect(page.getByText("You have unsaved changes.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Save changes" })).toHaveCount(1);
    await bar.getByRole("button", { name: "Save changes" }).click();
    await expect.poll(() => posts).toEqual(["/api/google/tools", "/api/google/grants"]);
  });

  test("a failed tool save stops before the asset save so no draft is lost", async ({ page }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { path: "/settings/data", posts, failToolSave: true });
    await draftAssetAndTool(page);
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByRole("alert").first()).toContainText("temporarily unavailable");
    expect(posts).toEqual(["/api/google/tools"]);
    await openSection(page, "Data access");
    await expect(
      page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" }),
    ).not.toBeChecked();
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toContainText(
      "1 asset change",
    );
  });

  test("Cancel discards both asset and tool drafts", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings/data" });
    await draftAssetAndTool(page);
    await page
      .getByRole("region", { name: "Unsaved changes" })
      .getByRole("button", { name: "Cancel" })
      .click();
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toHaveCount(0);
    await openSection(page, "Data access");
    await expect(page.getByRole("checkbox", { name: "Google Analytics 4 asset 1" })).toBeChecked();
  });

  test("switching account with unsaved tool changes asks before discarding", async ({ page }) => {
    await openHome(page, undefined, { path: "/settings/tools" });
    await page.getByRole("button", { name: /Google Analytics 4/ }).click();
    await toggle(page.getByRole("checkbox", { name: /^Run Report/ }));
    await chooseAccount(page, "empty@example.com");
    const dialog = page.getByRole("alertdialog", { name: "Discard unsaved changes?" });
    await dialog.getByRole("button", { name: "Keep editing" }).click();
    await expect(page.getByRole("button", { name: /Google account/ })).toContainText(
      "owner@example.com",
    );
  });
});

test.describe("@deterministic AI tools show only shared services by default (D03 taxonomy)", () => {
  test("tools for services the account doesn't share are hidden until asked for", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/settings/tools?account=conn-empty" });
    // conn-empty shares Analytics and Drive only, so the Tag Manager tool is hidden.
    await expect(page.getByRole("button", { name: /Google Tag Manager/ })).toHaveCount(0);
    await expect(page.locator(".tool-hidden-note")).toContainText("1 tool for 1 service");
    await page.getByRole("button", { name: "Show them" }).click();
    await expect(page.getByRole("button", { name: /Google Tag Manager/ })).toContainText(
      "Not shared",
    );
  });
});

test.describe("@deterministic accessibility beyond axe (#146 §3)", () => {
  test.describe("reflow at 320 CSS px (WCAG 1.4.10, equivalent to 400% zoom)", () => {
    test.use({ viewport: { width: 320, height: 640 } });
    for (const path of WORKSPACE_PAGES) {
      test(`${path} reflows without horizontal scrolling`, async ({ page }) => {
        const errors = await openHome(page, undefined, { path });
        expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
        expect(errors).toEqual([]);
      });
    }
  });

  test("keyboard only: skip link, sidebar, settings sections and the account picker", async ({
    page,
  }) => {
    await openHome(page, undefined, { path: "/settings/tools" });
    await page.keyboard.press("Tab");
    await expect(page.getByRole("link", { name: "Skip to content" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.locator("#main-content")).toBeFocused();
    // The settings sections come before the page content in tab order.
    const sections = page.getByRole("navigation", { name: "Settings sections" });
    await sections.getByRole("link", { name: /Data access/ }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("h1")).toHaveText("Data access");
    const picker = page.getByRole("button", { name: /Google account/ });
    await picker.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("listbox")).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/account=conn-empty/);
    await expect(picker).toBeFocused();
    // Every interactive element shows a visible focus indicator.
    await page.keyboard.press("Tab");
    const outline = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement;
      const style = getComputedStyle(el);
      return style.outlineStyle !== "none" || style.boxShadow !== "none";
    });
    expect(outline).toBe(true);
  });
});
