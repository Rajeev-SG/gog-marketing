import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

// Explicit browser fixture: native UI/assets and draft storage, not real auth/model proof.
test("@deterministic separate threads, legacy drafts, rename/delete and mobile shared navigation", async ({
  page,
}) => {
  const dist = fileURLToPath(new URL("../dist/client/", import.meta.url));
  const now = new Date().toISOString();
  type Thread = {
    id: string;
    title: string;
    conversation: string;
    createdAt: string;
    updatedAt: string;
  };
  const threads = new Map<string, Thread>([
    [
      "original",
      {
        id: "original",
        title: "Original conversation",
        conversation: "1".repeat(64),
        createdAt: now,
        updatedAt: now,
      },
    ],
  ]);
  const createdId = "00000000-0000-4000-8000-000000000002";
  await page.addInitScript(() => {
    Object.assign(window, {
      Clerk: {
        loaded: true,
        session: { id: "fixture-thread-session" },
        addListener(fn: (p: { session: { id: string } }) => void) {
          fn({ session: { id: "fixture-thread-session" } });
          return () => {};
        },
        async signOut() {},
      },
    });
  });
  await page.routeWebSocket("**/agents/**", () => {});
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    const method = route.request().method();
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
    if (url.pathname === "/api/chat/threads") {
      if (method === "POST") {
        const thread = {
          id: createdId,
          title: "New chat",
          conversation: "2".repeat(64),
          createdAt: now,
          updatedAt: now,
        };
        threads.set(createdId, thread);
        return route.fulfill({ status: 201, json: { thread } });
      }
      return route.fulfill({ json: { threads: [...threads.values()] } });
    }
    if (url.pathname.startsWith("/api/chat/threads/")) {
      const id = url.pathname.split("/").at(-1)!;
      const thread = threads.get(id)!;
      if (method === "PATCH") {
        thread.title = route.request().postDataJSON().title;
        return route.fulfill({ json: { thread } });
      }
      if (method === "DELETE") {
        threads.delete(id);
        return route.fulfill({ status: 204 });
      }
    }
    if (url.pathname === "/api/chat/session") {
      const thread =
        threads.get(url.searchParams.get("thread") ?? "original") ?? [...threads.values()][0];
      return route.fulfill({
        json: {
          thread,
          conversation: thread.conversation,
          draftKey: thread.conversation,
          model: "openai/gpt-4.1-mini",
          expiresAt: Date.now() + 3600000,
        },
      });
    }
    if (url.pathname.endsWith("/get-messages")) {
      const original = url.pathname.includes("1".repeat(64));
      return route.fulfill({
        json: [
          {
            id: original ? "legacy-reply" : "new-reply",
            role: "assistant",
            parts: [
              {
                type: "text",
                text: original ? "Legacy saved answer" : "Other conversation answer",
              },
            ],
          },
        ],
      });
    }
    return route.abort();
  });
  await page.goto("/chat");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(page.getByText("Legacy saved answer")).toBeVisible();
  await input.fill("original private draft");
  await page.getByRole("button", { name: "New chat", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(createdId));
  await expect(input).toHaveValue("");
  await expect(page.getByText("Legacy saved answer")).toHaveCount(0);
  await expect(page.getByText("Other conversation answer")).toBeVisible();
  await input.fill("new private draft");
  await page.reload();
  await expect(input).toHaveValue("new private draft");
  await page.getByRole("link", { name: "Original conversation", exact: true }).click();
  await expect(input).toHaveValue("original private draft");
  await expect(page.getByText("Legacy saved answer")).toBeVisible();
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await page.getByRole("textbox", { name: "Conversation name" }).fill("Renamed original");
  await page.getByRole("button", { name: "Save name", exact: true }).click();
  await expect(page.getByRole("link", { name: "Renamed original", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByText("Legacy saved answer")).toBeVisible();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Delete conversation", exact: true }).click();
  await expect(page.getByRole("link", { name: "Renamed original", exact: true })).toHaveCount(0);
  await expect(input).toHaveValue("new private draft");
  await expect(page.getByText("Other conversation answer")).toBeVisible();
  expect(
    await page.evaluate(() => sessionStorage.getItem("gog:chat:draft:" + "1".repeat(64))),
  ).toBeNull();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page
      .getByRole("navigation", { name: "Mobile navigation", exact: true })
      .getByRole("link", { name: "Google data" }),
  ).toHaveAttribute("href", "/#google-data");
  await page.getByRole("button", { name: "Open conversations" }).click();
  await expect(page.getByRole("button", { name: "New chat", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "Open conversations" })).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
