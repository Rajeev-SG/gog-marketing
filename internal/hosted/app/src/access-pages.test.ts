import vm from "node:vm";
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

it("clears the loader before native mount and constrains the provider heading typography", async () => {
  const nodes = new Map<
    string,
    { hidden: boolean; textContent: string; addEventListener: () => void }
  >();
  const element = (id: string) => {
    if (!nodes.has(id))
      nodes.set(id, {
        hidden: id !== "sign-in",
        textContent: id === "sign-in" ? "Secure sign-in is loading…" : "",
        addEventListener: () => {},
      });
    return nodes.get(id)!;
  };
  let load: (() => Promise<void>) | undefined;
  let mounted = false;
  const context = vm.createContext({
    document: { getElementById: element },
    window: {
      addEventListener: (_event: string, fn: () => Promise<void>) => {
        load = fn;
      },
    },
    Clerk: {
      load: async () => {},
      mountSignIn: (
        target: ReturnType<typeof element>,
        options: { appearance: { elements: Record<string, Record<string, unknown>> } },
      ) => {
        expect(target.textContent).toBe("");
        expect(options.appearance.elements.headerTitle).toMatchObject({
          fontSize: "1.25rem",
          lineHeight: "1.4",
          letterSpacing: "normal",
          margin: 0,
        });
        expect(options.appearance.elements.headerSubtitle.fontSize).toBe("0.875rem");
        mounted = true;
      },
    },
    setTimeout: () => 1,
    clearTimeout: () => {},
  });
  vm.runInContext("window.Clerk=Clerk", context);
  const script = renderSignIn(config).match(/<script>([\s\S]*?)<\/script>/)![1];
  vm.runInContext(script, context);
  await load!();
  expect(mounted).toBe(true);
  expect(element("signin-error").hidden).toBe(true);
});

it("exposes sign-in retry after a stalled SDK without late mounting", async () => {
  const nodes = new Map<string, { hidden: boolean; addEventListener: () => void }>();
  const element = (id: string) => {
    if (!nodes.has(id)) nodes.set(id, { hidden: id !== "sign-in", addEventListener: () => {} });
    return nodes.get(id)!;
  };
  let load: (() => Promise<void>) | undefined;
  let timeout: (() => void) | undefined;
  let resolveLoad: (() => void) | undefined;
  let mounted = false;
  const context = vm.createContext({
    document: { getElementById: element },
    window: {
      addEventListener: (_event: string, fn: () => Promise<void>) => {
        load = fn;
      },
    },
    Clerk: {
      load: () =>
        new Promise<void>((resolve) => {
          resolveLoad = resolve;
        }),
      mountSignIn: () => {
        mounted = true;
      },
    },
    setTimeout: (fn: () => void, ms: number) => {
      expect(ms).toBe(15000);
      timeout = fn;
      return 1;
    },
    clearTimeout: () => {},
  });
  vm.runInContext("window.Clerk=Clerk", context);
  const script = renderSignIn(config).match(/<script>([\s\S]*?)<\/script>/)![1];
  vm.runInContext(script, context);
  const pending = load!();
  timeout!();
  expect(element("signin-error").hidden).toBe(false);
  expect(element("sign-in").hidden).toBe(true);
  resolveLoad!();
  await pending;
  expect(mounted).toBe(false);
});
