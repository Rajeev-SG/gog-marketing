import { defineConfig, devices } from "@playwright/test";

const liveBaseUrl = process.env.PLAYWRIGHT_BASE_URL;
const fixturePort = process.env.PLAYWRIGHT_PORT ?? "8788";
const baseURL = liveBaseUrl ?? `http://127.0.0.1:${fixturePort}`;
const systemChrome = process.env.PLAYWRIGHT_USE_SYSTEM_CHROME === "1";
const browserOverrides = systemChrome ? { channel: "chrome" as const } : {};

export default defineConfig({
  testDir: ".",
  testMatch: "owner-journey.spec.ts",
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  outputDir: "test-results",
  reporter: [["list"]],
  use: {
    baseURL,
    // Owner journeys exercise real Clerk credentials; trace.zip can retain
    // auth cookies, headers, and redirected auth payloads.
    trace: "off",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], ...browserOverrides },
    },
  ],
  webServer: liveBaseUrl
    ? undefined
    : {
        command: "node ./fixture-server.mjs",
        url: baseURL,
        reuseExistingServer: true,
        timeout: 30_000,
      },
});
