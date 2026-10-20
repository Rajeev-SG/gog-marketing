import { expect, it } from "vitest";
import {
  renderLanding,
  renderRequestAccess,
  renderAccessState,
  safeWorkspaceReturn,
} from "./access-pages.js";
import { renderSignIn } from "./html.js";
const config = {
  publishableKey: "pk_test_fixture",
  clerkDomain: "fixture.clerk.accounts.dev",
  authorizedParties: ["https://app.test"],
  mcpOrigin: "https://app.test",
};
it("renders a public marketer entry without auth SDK, private workspace controls or admission", () => {
  const html = renderLanding();
  expect(html).toContain('href="/sign-in"');
  expect(html).toContain("Docs, Sheets, Slides and Drive");
  expect(html).toContain("Invitation-only beta");
  expect(html).not.toContain("clerk-js");
  expect(html).not.toContain('id="app-shell"');
  expect(html).not.toContain("mountSignUp");
  expect(renderRequestAccess()).toContain("No request has been submitted");
  expect(renderRequestAccess()).not.toContain("<form");
});
it("separates denied, ended-session and provider failure pages with safe next actions", () => {
  expect(renderAccessState("not-invited")).toContain("This workspace isn’t available yet");
  expect(renderAccessState("expired")).toContain("expired=1");
  expect(renderAccessState("unavailable")).toContain("Try again");
  for (const kind of ["not-invited", "expired", "unavailable"] as const) {
    const html = renderAccessState(kind);
    expect(html).toContain('class="access-page"');
    expect(html).not.toContain("clerk-js");
    expect(html).not.toContain('id="app-shell"');
  }
});
it("retains native Clerk with brand appearance, bounded retry and literal safe return targets", () => {
  const html = renderSignIn(config, "/chat?thread=original");
  expect(html).toContain("Clerk.mountSignIn");
  expect(html).toContain('"fallbackRedirectUrl":"/chat?thread=original"');
  expect(html).toContain('"colorPrimary":"var(--action)"');
  expect(html).toContain('id="signin-error"');
  expect(html).toContain('id="signin-retry"');
  expect(html).not.toContain("mountSignUp");
  for (const bad of [
    "https://evil.test/",
    "//evil.test/",
    "/chat?thread=one&redirect=https://evil.test",
    "/sign-in",
    "/chat#<script>",
  ]) {
    expect(safeWorkspaceReturn(bad)).toBe("/");
    expect(renderSignIn(config, bad)).not.toContain('"fallbackRedirectUrl":"' + bad + '"');
  }
});
