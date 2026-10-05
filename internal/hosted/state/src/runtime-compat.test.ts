/**
 * Production-graph compatibility tests.
 *
 * 1. The bundled production entrypoint must not depend on Node built-ins
 *    (Worker-compatible bundle proof).
 * 2. The bundled repository must run inside workerd against a real D1
 *    binding (runtime proof, exercised with Miniflare).
 */
import { afterEach, describe, expect, it } from "vitest";
import { build } from "esbuild";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { splitMigrationStatements } from "./dev-migrations.js";

const MIGRATION_FILE = "0001_initial_schema.sql";

async function bundleRuntimeWorker(): Promise<string> {
  const result = await build({
    entryPoints: [join(import.meta.dirname, "worker-runtime.ts")],
    bundle: true,
    format: "esm",
    platform: "browser",
    target: "es2022",
    write: false,
    logLevel: "silent",
  });
  return result.outputFiles[0].text;
}

describe("worker bundle compatibility", () => {
  it("bundles src/index.ts without Node built-ins or CJS requires", async () => {
    const result = await build({
      entryPoints: [join(import.meta.dirname, "index.ts")],
      bundle: true,
      format: "esm",
      platform: "browser",
      target: "es2022",
      write: false,
      logLevel: "silent",
    });

    const code = result.outputFiles.map((f) => f.text).join("\n");
    expect(code).not.toMatch(/node:(fs|path|url|sqlite|module)/);
    expect(code).not.toMatch(/\brequire\(/);
    expect(code).not.toMatch(/\b__dirname\b/);
  });
});

describe("hosted state in workerd (native D1 runtime)", () => {
  let mf: Miniflare | undefined;

  afterEach(async () => {
    await mf?.dispose();
    mf = undefined;
  });

  it("bootstraps tenants, rounds BLOB credentials through workerd, and increments quota concurrently", async () => {
    const runtimeWorkerSource = await bundleRuntimeWorker();
    mf = new Miniflare(
      convertV4MiniflareOptions({
        workers: [
          {
            name: "hosted-state-runtime",
            compatibilityDate: "2024-12-01",
            d1Databases: { DB: "hosted-state-test" },
            script: runtimeWorkerSource,
            modules: true,
          },
        ],
      }),
    );

    const db = await mf.getD1Database("DB");

    // Apply the real migration file through the native D1 API before the
    // runtime worker performs any repository operations.
    const migrationSql = readFileSync(
      join(import.meta.dirname, "..", "migrations", MIGRATION_FILE),
      "utf-8",
    );
    for (const statement of splitMigrationStatements(migrationSql)) {
      const statementText = statement.trim();
      if (!statementText) continue;
      const result = await db.batch([db.prepare(statementText)]);
      if (!result[0]?.success) throw new Error(`migration statement failed: ${statementText}`);
    }

    // Tenant bootstrap inside workerd.
    const tenant = (await (
      await mf.dispatchFetch("https://hosted.test/bootstrap", {
        method: "POST",
        body: JSON.stringify({ clerkUserId: "clerk_runtime" }),
      })
    ).json()) as { id: string; clerkUserId: string };
    expect(tenant.clerkUserId).toBe("clerk_runtime");
    expect(tenant.id).toBeTruthy();

    // Connection + credential roundtrip inside workerd. The repository must
    // normalise native BLOB reads to byte arrays; the raw D1 read-back is
    // asserted separately in native-d1.test.ts.
    const conn = (await (
      await mf.dispatchFetch("https://hosted.test/conn", {
        method: "POST",
        body: JSON.stringify({ tenantId: tenant.id }),
      })
    ).json()) as { id: string; tenantId: string };
    expect(conn.tenantId).toBe(tenant.id);

    const ciphertext = [0x01, 0x02, 0x03, 0x04, 0xff];
    const nonce = [0x10, 0x20, 0x30];
    const stored = (await (
      await mf.dispatchFetch("https://hosted.test/cred-store", {
        method: "POST",
        body: JSON.stringify({
          tenantId: tenant.id,
          connectionId: conn.id,
          ciphertext,
          nonce,
          keyVersion: 3,
        }),
      })
    ).json()) as { ok: boolean };
    expect(stored.ok).toBe(true);

    const readBack = (await (
      await mf.dispatchFetch("https://hosted.test/cred-read", {
        method: "POST",
        body: JSON.stringify({ tenantId: tenant.id, connectionId: conn.id }),
      })
    ).json()) as { ciphertext: number[] | null; keyVersion: number | null };
    expect(readBack.ciphertext).toEqual(ciphertext);
    expect(readBack.keyVersion).toBe(3);

    // Concurrent quota increments inside workerd return exact 1..N.
    const callers = 16;
    const values = await Promise.all(
      Array.from({ length: callers }, () =>
        mf!
          .dispatchFetch("https://hosted.test/quota", {
            method: "POST",
            body: JSON.stringify({
              tenantId: tenant.id,
              period: "2026-10",
              counter: "calls",
              delta: 1,
            }),
          })
          .then((r) => r.json() as Promise<{ value: number }>),
      ),
    );
    expect(values.map((v) => v.value).sort((a, b) => a - b)).toEqual(
      Array.from({ length: callers }, (_, i) => i + 1),
    );
  });
});
