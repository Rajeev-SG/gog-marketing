/**
 * Test-only worker entrypoint used to prove the production repository runs
 * inside workerd against a real D1 binding. Bundled with esbuild in tests;
 * never imported by `src/index.ts` or the test suites.
 */
import { HostedRepository } from "./repository.js";

export interface RuntimeEnv {
  DB: D1Database;
}

export default {
  async fetch(request: Request, env: RuntimeEnv): Promise<Response> {
    const repo = new HostedRepository(env.DB);
    const body = (await request.json()) as Record<string, unknown>;

    switch (new URL(request.url).pathname) {
      case "/bootstrap":
        return Response.json(await repo.bootstrapTenant(String(body.clerkUserId)));
      case "/quota": {
        const value = await repo.incrementQuota(
          String(body.tenantId),
          String(body.period),
          String(body.counter),
          Number(body.delta),
        );
        return Response.json({ value });
      }
      case "/conn": {
        return Response.json(
          await repo.createConnection(
            String(body.tenantId),
            "sub_runtime",
            "runtime@gmail.com",
            "",
            "[]",
          ),
        );
      }
      case "/cred-store": {
        await repo.upsertCredential({
          tenantId: String(body.tenantId),
          connectionId: String(body.connectionId),
          ciphertext: new Uint8Array(body.ciphertext as number[]),
          nonce: new Uint8Array(body.nonce as number[]),
          keyVersion: Number(body.keyVersion),
        });
        return Response.json({ ok: true });
      }
      case "/cred-read": {
        const stored = await repo.getCredential(String(body.tenantId), String(body.connectionId));
        return Response.json({
          ciphertext: stored ? Array.from(stored.ciphertext) : null,
          nonce: stored ? Array.from(stored.nonce) : null,
          keyVersion: stored?.keyVersion ?? null,
        });
      }
      default:
        return new Response("not found", { status: 404 });
    }
  },
};
