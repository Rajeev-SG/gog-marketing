import { afterAll, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import { generateText } from "ai";
import { HostedRepository } from "../../../state/src/repository.js";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { budgetedModelFetch } from "./model-fetch.js";
import { modelProviderSettings, selectChatModel, type ModelFailure } from "./models.js";
let mf: Miniflare, repo: HostedRepository, tenantId: string;
beforeAll(async () => {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "model-fetch",
          modules: true,
          script: "export default {};",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "model-fetch" },
        },
      ],
    }),
  );
  const db = await mf.getD1Database("DB");
  for (const file of ["0001_initial_schema.sql", "0003_rate_limit_counters.sql"])
    for (const sql of splitMigrationStatements(loadMigrationSql(file))) await db.prepare(sql).run();
  repo = new HostedRepository(db as unknown as D1Database);
  tenantId = (await repo.bootstrapTenant("model-fetch-owner")).id;
});
beforeEach(async () => {
  await repo.db.prepare("DELETE FROM hosted_rate_limit_counters").run();
});
afterAll(async () => {
  await mf?.dispose();
});
const env = {
  CHAT_OWNER_USER_ID: "owner",
  CHAT_PAID_MODELS_ENABLED: "true",
  CHAT_TENANT_MONTHLY_BUDGET_MICRO_USD: "250000",
  CHAT_GLOBAL_MONTHLY_BUDGET_MICRO_USD: "1000000",
};
const completion = (model: string) =>
  Response.json({
    id: "synthetic",
    created: 1,
    model,
    choices: [{ index: 0, finish_reason: "stop", message: { role: "assistant", content: "OK" } }],
    usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
  });
function options(overrides: Partial<Parameters<typeof budgetedModelFetch>[0]> = {}) {
  return {
    repo,
    tenantId,
    userId: "owner",
    model: selectChatModel("free-router", {}, "owner"),
    env: {},
    apiKey: "synthetic",
    authorize: async () => {},
    selectionCurrent: async () => true,
    failed: (_code: ModelFailure) => {},
    ...overrides,
  };
}
it("counts every actual SDK request across model choices and stops at the application cap", async () => {
  const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    expect(init?.redirect).toBe("manual");
    return completion("openrouter/free");
  });
  const first = createOpenRouter({
    apiKey: "synthetic",
    fetch: budgetedModelFetch(options({ fetcher: fetcher as typeof fetch })),
  });
  const model = selectChatModel("free-router", {}, "owner");
  await generateText({
    model: first.chat(model.id, modelProviderSettings(model)),
    prompt: "hello",
    maxOutputTokens: 16,
    maxRetries: 0,
  });
  expect(fetcher).toHaveBeenCalledTimes(1);
  await repo.db
    .prepare(
      `UPDATE hosted_rate_limit_counters SET value=50 WHERE scope_key='gog-chat:global' AND period LIKE 'model-requests-day:%'`,
    )
    .run();
  const failures: ModelFailure[] = [];
  const other = selectChatModel("minimax-free", {}, "owner");
  const second = createOpenRouter({
    apiKey: "synthetic",
    fetch: budgetedModelFetch(
      options({
        model: other,
        fetcher: fetcher as typeof fetch,
        failed: (code) => failures.push(code),
      }),
    ),
  });
  await expect(
    generateText({
      model: second.chat(other.id, modelProviderSettings(other)),
      prompt: "same account, different model",
      maxOutputTokens: 16,
      maxRetries: 0,
    }),
  ).rejects.toThrow();
  expect(fetcher).toHaveBeenCalledTimes(1);
  expect(failures).toContain("budget");
});
it("rechecks session and current selection after reservation and never dispatches stale authorization", async () => {
  const fetcher = vi.fn(async () => completion("openrouter/free"));
  let checks = 0;
  const model = selectChatModel("free-router", {}, "owner");
  const fetch = budgetedModelFetch(
    options({
      fetcher: fetcher as typeof globalThis.fetch,
      selectionCurrent: async () => ++checks === 1,
    }),
  );
  await expect(
    fetch("https://openrouter.ai/api/v1/chat/completions", {
      method: "POST",
      body: JSON.stringify({
        model: model.id,
        max_tokens: 16,
        messages: [{ role: "user", content: "hello" }],
        ...modelProviderSettings(model),
      }),
    }),
  ).rejects.toThrow();
  expect(checks).toBe(2);
  expect(fetcher).not.toHaveBeenCalled();
});
it("permits an explicitly authorized paid request only under both provider and local hard caps", async () => {
  const model = selectChatModel("gpt41-mini", env, "owner", true);
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input).endsWith("/key"))
      return Response.json({
        data: {
          limit: 0.25,
          limit_remaining: 0.25,
          limit_reset: "monthly",
          include_byok_in_limit: true,
        },
      });
    const payload = JSON.parse(String(init?.body));
    expect(payload.provider.only).toEqual(["openai"]);
    expect(payload.provider.max_price).toMatchObject({ prompt: 0.4, completion: 1.6, request: 0 });
    return completion(model.id);
  });
  const provider = createOpenRouter({
    apiKey: "synthetic",
    fetch: budgetedModelFetch(options({ env, model, fetcher: fetcher as typeof fetch })),
  });
  await generateText({
    model: provider.chat(model.id, modelProviderSettings(model)),
    prompt: "synthetic paid-policy fixture, not a real paid call",
    maxOutputTokens: 16,
    maxRetries: 0,
  });
  expect(fetcher).toHaveBeenCalledTimes(2);
  const reserved = await repo.db
    .prepare(
      "SELECT value FROM hosted_rate_limit_counters WHERE scope_key=?1 AND period LIKE 'model-cost-micro-usd-month:%'",
    )
    .bind("gog-chat:tenant:" + tenantId)
    .first<{ value: number }>();
  expect(reserved!.value).toBeGreaterThan(0);
  const before = fetcher.mock.calls.length;
  const forbidden = budgetedModelFetch(
    options({ env, model, userId: "cohort", fetcher: fetcher as typeof fetch }),
  );
  await expect(
    forbidden("https://openrouter.ai/api/v1/chat/completions", {
      method: "POST",
      body: JSON.stringify({
        model: model.id,
        max_tokens: 16,
        messages: [],
        ...modelProviderSettings(model),
      }),
    }),
  ).rejects.toThrow();
  expect(fetcher.mock.calls.length).toBe(before);
});
it("returns safe privacy/rate errors with one SDK attempt and never follows redirects or pays for fallback", async () => {
  const model = selectChatModel("free-router", {}, "owner");
  for (const status of [404, 429, 302]) {
    const failures: ModelFailure[] = [];
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.redirect).toBe("manual");
      return new Response(JSON.stringify({ error: { message: "synthetic unavailable" } }), {
        status,
        headers: { "Content-Type": "application/json", Location: "https://never-follow.example/" },
      });
    });
    const provider = createOpenRouter({
      apiKey: "synthetic",
      fetch: budgetedModelFetch(
        options({ fetcher: fetcher as typeof fetch, failed: (code) => failures.push(code) }),
      ),
    });
    await expect(
      generateText({
        model: provider.chat(model.id, modelProviderSettings(model)),
        prompt: "fixture",
        maxOutputTokens: 16,
        maxRetries: 0,
      }),
    ).rejects.toThrow();
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(failures).toContain(status === 404 ? "privacy" : status === 429 ? "budget" : "provider");
  }
});
