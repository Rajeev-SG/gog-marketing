import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
    // Native workerd/D1 startup shares the Oracle CI CPU with parallel jobs.
    testTimeout: 20_000,
    hookTimeout: 20_000,
  },
});
