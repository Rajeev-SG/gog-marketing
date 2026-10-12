import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
    // Native D1/Clerk fixtures share the two-CPU Oracle host with Go builds.
    // Keep assertions and finite deadlines; local and application defaults stay unchanged.
    testTimeout: process.env.CI ? 20_000 : 5_000,
    hookTimeout: process.env.CI ? 20_000 : 10_000,
  },
});
