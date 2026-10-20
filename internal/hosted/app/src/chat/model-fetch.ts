import { HostedRepository } from "../../../state/src/repository.js";
import { reserveModelRequest, DEFAULT_MODEL_REQUEST_LIMITS } from "./model-budgets.js";
import {
  ChatModelError,
  modelRequestReservation,
  paidModelCaps,
  paidModelConfigured,
  paidProviderCapLimit,
  verifyPaidProviderBudget,
  type ChatModelEnv,
  type ChatModelPolicy,
  type ModelFailure,
} from "./models.js";

/** Native SDK fetch middleware, not a replacement streaming implementation. */
export function budgetedModelFetch(options: {
  repo: HostedRepository;
  tenantId: string;
  userId: string;
  model: ChatModelPolicy;
  env: ChatModelEnv;
  apiKey: string;
  authorize: () => Promise<void>;
  selectionCurrent: () => Promise<boolean>;
  failed: (code: ModelFailure) => void;
  fetcher?: typeof fetch;
}): typeof fetch {
  const underlying = options.fetcher ?? fetch;
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    const fail = (code: ModelFailure): never => {
      options.failed(code);
      throw new ChatModelError(code);
    };
    await options.authorize();
    if (!(await options.selectionCurrent())) return fail("model_not_allowed");
    const url = new URL(
      typeof input === "string" ? input : input instanceof URL ? input.href : input.url,
    );
    if (
      url.origin !== "https://openrouter.ai" ||
      url.pathname !== "/api/v1/chat/completions" ||
      url.username ||
      url.password ||
      url.search ||
      init?.method?.toUpperCase() !== "POST" ||
      typeof init.body !== "string"
    )
      return fail("model_not_allowed");
    let reservation;
    try {
      reservation = modelRequestReservation(init.body, options.model);
    } catch (error) {
      return fail(error instanceof ChatModelError ? error.code : "model_not_allowed");
    }
    const caps = paidModelCaps(options.env);
    if (reservation.paid) {
      if (!paidModelConfigured(options.env, options.userId)) return fail("paid_not_enabled");
      const providerBudget = await verifyPaidProviderBudget(
        options.apiKey,
        paidProviderCapLimit(options.env),
        underlying,
      );
      if (!providerBudget) return fail("paid_not_enabled");
      if (providerBudget.remainingMicroUsd < reservation.costMicroUsd) return fail("budget");
    }
    let reserved = false;
    try {
      reserved = await reserveModelRequest(options.repo, options.tenantId, reservation, {
        ...DEFAULT_MODEL_REQUEST_LIMITS,
        tenantMonthCostMicroUsd: caps.tenant,
        globalMonthCostMicroUsd: caps.global,
      });
    } catch {
      return fail("budget");
    }
    if (!reserved) return fail("budget");
    // A revocation or model switch during a D1/provider preflight still prevents dispatch.
    await options.authorize();
    if (!(await options.selectionCurrent())) return fail("model_not_allowed");
    let response: Response;
    try {
      response = await underlying(input, { ...init, redirect: "manual" });
    } catch {
      return fail("provider");
    }
    if (!response.ok) {
      options.failed(
        response.status === 429 || response.status === 402 || response.status === 403
          ? "budget"
          : response.status === 404
            ? "privacy"
            : "provider",
      );
    }
    return response;
  }) as typeof fetch;
}
