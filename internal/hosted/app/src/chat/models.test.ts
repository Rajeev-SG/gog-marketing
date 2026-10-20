import { expect, it, vi } from "vitest";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import { generateText } from "ai";
import {
  availableChatModels,
  DEFAULT_CHAT_MODEL,
  modelProviderSettings,
  modelRequestReservation,
  paidModelConfigured,
  selectChatModel,
  verifyPaidProviderBudget,
} from "./models.js";
const env = {
  CHAT_OWNER_USER_ID: "owner",
  CHAT_PAID_MODELS_ENABLED: "true",
  CHAT_TENANT_MONTHLY_BUDGET_MICRO_USD: "250000",
  CHAT_GLOBAL_MONTHLY_BUDGET_MICRO_USD: "1000000",
};
it("uses a server-owned free default, hides paid from cohorts and rejects forged model ids", () => {
  expect(DEFAULT_CHAT_MODEL).toBe("openrouter/free");
  expect(availableChatModels({}, "cohort").every((x) => x.free)).toBe(true);
  expect(
    availableChatModels({}, "cohort")
      .filter((x) => x.enabled)
      .map((x) => x.key),
  ).toEqual(["free-router", "xing-free"]);
  expect(availableChatModels(env, "owner").find((x) => !x.free)?.enabled).toBe(false);
  expect(() => selectChatModel("openai/gpt-4.1-mini", env, "owner", true)).toThrow();
  expect(() => selectChatModel("gpt41-mini", env, "cohort", true)).toThrow();
  expect(() => selectChatModel("gpt41-mini", {}, "owner", true)).toThrow();
  expect(selectChatModel("gpt41-mini", env, "owner", true).free).toBe(false);
  expect(paidModelConfigured({ ...env, CHAT_GLOBAL_MONTHLY_BUDGET_MICRO_USD: "-1" }, "owner")).toBe(
    false,
  );
});
it("requires an existing provider hard cap without changing its settings", async () => {
  const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
    Response.json({
      data: { limit: 1, limit_remaining: 0.5, limit_reset: "monthly", include_byok_in_limit: true },
    }),
  );
  expect(await verifyPaidProviderBudget("synthetic", 1000000, fetcher as typeof fetch)).toEqual({
    capMicroUsd: 1000000,
    remainingMicroUsd: 500000,
  });
  expect(fetcher.mock.calls[0]?.[0]).toBe("https://openrouter.ai/api/v1/key");
  for (const data of [
    { limit: null, limit_remaining: 1 },
    { limit: 2, limit_remaining: 2, limit_reset: "monthly", include_byok_in_limit: true },
    { limit: 1, limit_remaining: 1, limit_reset: "daily", include_byok_in_limit: true },
    { limit: 1, limit_remaining: 1, limit_reset: "monthly", include_byok_in_limit: false },
  ]) {
    expect(
      await verifyPaidProviderBudget("synthetic", 1000000, (async () =>
        Response.json({ data })) as typeof fetch),
    ).toBeNull();
  }
});
it("serializes privacy and zero-price controls through the installed SDK, never requesting a paid fallback", async () => {
  const model = selectChatModel("free-router", {}, "cohort");
  const fetcher = vi.fn(async (_url: unknown, init?: RequestInit) => {
    const payload = JSON.parse(String(init?.body));
    const reservation = modelRequestReservation(String(init?.body), model);
    expect(reservation.costMicroUsd).toBe(0);
    expect(payload.models).toBeUndefined();
    expect(payload.provider).toMatchObject({
      zdr: true,
      data_collection: "deny",
      allow_fallbacks: false,
      max_price: { prompt: 0, completion: 0, request: 0, image: 0, audio: 0 },
    });
    return Response.json({
      id: "fixture",
      created: 1,
      model: DEFAULT_CHAT_MODEL,
      choices: [{ index: 0, finish_reason: "stop", message: { role: "assistant", content: "OK" } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    });
  });
  const provider = createOpenRouter({ apiKey: "synthetic", fetch: fetcher as typeof fetch });
  await generateText({
    model: provider.chat(model.id, modelProviderSettings(model)),
    prompt: "hello",
    maxOutputTokens: 32,
    maxRetries: 0,
  });
  expect(fetcher).toHaveBeenCalledTimes(1);
});
it("rejects relaxed privacy, media, forged fallback and uncapped paid routing before dispatch", () => {
  const model = selectChatModel("free-router", {}, "cohort");
  const valid = {
    model: model.id,
    max_tokens: 32,
    messages: [{ role: "user", content: "hello" }],
    ...modelProviderSettings(model),
  };
  for (const bad of [
    { ...valid, models: ["paid"] },
    { ...valid, provider: { zdr: false } },
    {
      ...valid,
      messages: [
        {
          role: "user",
          content: [{ type: "image_url", image_url: { url: "https://example.test/private" } }],
        },
      ],
    },
    { ...valid, max_tokens: 99999 },
  ])
    expect(() => modelRequestReservation(JSON.stringify(bad), model)).toThrow();
  const paid = selectChatModel("gpt41-mini", env, "owner", true);
  const reservation = modelRequestReservation(
    JSON.stringify({ ...valid, model: paid.id, ...modelProviderSettings(paid) }),
    paid,
  );
  expect(reservation.paid).toBe(true);
  expect(reservation.costMicroUsd).toBeGreaterThan(0);
});

it("disables known privacy-incompatible free choices without relaxing policy or silently changing a thread", () => {
  for (const key of ["minimax-free", "nemotron-free"]) {
    const choice = availableChatModels({}, "cohort").find((model) => model.key === key);
    expect(choice).toMatchObject({ key, free: true, enabled: false });
    expect(choice?.disabledReason).toContain("zero-data-retention");
    expect(() => selectChatModel(key, {}, "cohort")).toThrow("No eligible provider");
  }
  const model = selectChatModel("xing-free", {}, "cohort");
  expect(model.id).toBe("xingchen-agi/xing4.0-29b-a4b:free");
  expect(modelProviderSettings(model).provider).toMatchObject({
    zdr: true,
    data_collection: "deny",
    allow_fallbacks: false,
    require_parameters: true,
    max_price: { prompt: 0, completion: 0, request: 0, image: 0, audio: 0 },
  });
});
