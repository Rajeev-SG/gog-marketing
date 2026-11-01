import { expect, test } from "@playwright/test";
import {
  LIMIT_ERROR,
  axeViolations,
  horizontalOverflow,
  openChat,
  openHome,
} from "./audit-fixtures.js";

// Regression checks for the 31 Oct 2026 UX audit (FINDINGS.md) and design
// review (DESIGN-REVIEW.md). Each test names the finding IDs it guards.

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

  test.describe(`@deterministic home at ${viewport.name}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test("no horizontal overflow, no page errors and axe clean", async ({ page }) => {
      const errors = await openHome(page);
      expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
      expect(errors).toEqual([]);
      expect(await axeViolations(page)).toEqual([]);
    });
  });
}

test.describe("@deterministic home account context (U03, D01)", () => {
  test("switching to an empty account resets the selection summary and names the account", async ({
    page,
  }) => {
    await openHome(page);
    const workspace = page.locator("#asset-workspace");
    await expect(workspace).toBeVisible();
    await expect(page.locator("#asset-selected-count")).toHaveText("3 selected");
    await page
      .locator(".account-card", { hasText: "empty@example.com" })
      .getByRole("button", { name: /manage data access/i })
      .click();
    await expect(workspace.getByText("No assets found yet")).toBeVisible();
    await expect(page.locator("#asset-selected-count")).toHaveText("0 selected");
    await expect(page.locator("#asset-health")).not.toContainText("All services healthy");
    await expect(workspace).toContainText("empty@example.com");
  });
});

test.describe("@deterministic home overview (D02, L01, L03, N04)", () => {
  test("service rows count every granting account and open the asset picker", async ({ page }) => {
    await openHome(page);
    const analytics = page.locator(".service-row", { hasText: "Google Analytics" });
    // Both fixture accounts granted Analytics; only one has discovered assets.
    await expect(analytics).toContainText("2 accounts");
    await expect(analytics).toContainText("2 of 2");
    await expect(page.locator(".service-row", { hasText: "Google Tag Manager" })).toContainText(
      "1 account",
    );
    await page.locator(".service-row", { hasText: "Google Tag Manager" }).click();
    await expect(page.locator("#asset-workspace .card-head h3")).toHaveText("Google Tag Manager");
    await expect(page.locator(".service-mark").first()).toHaveAttribute("aria-hidden", "true");
  });

  test("the setup banner shows only while setup is incomplete", async ({ page }) => {
    await openHome(page);
    await expect(page.locator("#setup-guide")).toBeVisible();
    await expect(page.locator("#setup-next")).toHaveText("Choose AI tools");
  });

  test("the setup banner is hidden once every step is complete", async ({ page }) => {
    await openHome(page, undefined, { toolsEnabled: true });
    await expect(page.locator("#tool-summary")).toContainText("1 enabled");
    await expect(page.locator("#setup-guide")).toBeHidden();
  });

  test("account menu closes on Escape, names its account and returns focus", async ({ page }) => {
    await openHome(page);
    const card = page.locator(".account-card", { hasText: "owner@example.com" });
    const summary = card.locator("summary");
    await summary.click();
    await expect(card.locator(".context-menu-header")).toHaveText("Actions for owner@example.com");
    await page.keyboard.press("Escape");
    await expect(card.locator("details.context-menu")).not.toHaveAttribute("open", "");
    await expect(summary).toBeFocused();
    await summary.click();
    await page.locator("h1").click();
    await expect(card.locator("details.context-menu")).not.toHaveAttribute("open", "");
  });
});

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

test.describe("@deterministic asset picker (DR09, L06)", () => {
  test("footer summarises the selection, Cancel discards, search shows filtered counts", async ({
    page,
  }) => {
    await openHome(page);
    await expect(page.locator("#asset-footer-count")).toHaveText("3 assets selected");
    const first = page.getByRole("checkbox", { name: /asset 1 access/ });
    await first.uncheck();
    await expect(page.locator("#asset-footer-count")).toHaveText("2 assets selected");
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(first).toBeChecked();
    await expect(page.locator("#asset-footer-count")).toHaveText("3 assets selected");
    await page.getByRole("searchbox", { name: "Search assets" }).fill("asset 2");
    await expect(page.locator(".service-meta")).toHaveText(/Showing 1 of 2 assets matching/);
  });
});

test.describe("@deterministic one save model (D03 decision A)", () => {
  test("assets and tools share one save bar and save with one click", async ({ page }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { posts });
    await expect(page.locator(".savebar")).toHaveCount(1);
    await expect(page.getByRole("button", { name: /save tool permissions/i })).toHaveCount(0);
    await expect(page.locator("#pending-summary")).toHaveText("No unsaved changes");
    await expect(page.locator("#access-savebar")).toBeHidden();
    await page.getByRole("checkbox", { name: /asset 1 access/ }).uncheck();
    await expect(page.locator("#access-savebar")).toBeVisible();
    await page.locator(".tool-group summary").first().click();
    await page.locator('input[data-tool-name="analytics_run_report"]').check();
    await expect(page.locator("#pending-summary")).toHaveText(
      "Unsaved: 1 asset change and tool permission changes",
    );
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect.poll(() => posts).toEqual(["/api/google/tools", "/api/google/grants"]);
  });

  test("a failed tool save stops before the asset save so no draft is lost", async ({ page }) => {
    const posts: string[] = [];
    await openHome(page, undefined, { posts, failToolSave: true });
    await page.getByRole("checkbox", { name: /asset 1 access/ }).uncheck();
    await page.locator(".tool-group summary").first().click();
    await page.locator('input[data-tool-name="analytics_run_report"]').check();
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.locator("#tool-status")).toContainText("temporarily unavailable");
    expect(posts).toEqual(["/api/google/tools"]);
    await expect(page.getByRole("checkbox", { name: /asset 1 access/ })).not.toBeChecked();
    await expect(page.locator("#pending-summary")).toContainText("1 asset change");
  });

  test("Cancel discards both asset and tool drafts", async ({ page }) => {
    await openHome(page);
    await page.getByRole("checkbox", { name: /asset 1 access/ }).uncheck();
    await page.locator(".tool-group summary").first().click();
    await page.locator('input[data-tool-name="analytics_run_report"]').check();
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(page.locator("#pending-summary")).toHaveText("No unsaved changes");
    await expect(page.locator("#access-savebar")).toBeHidden();
    await expect(page.getByRole("checkbox", { name: /asset 1 access/ })).toBeChecked();
  });
});
