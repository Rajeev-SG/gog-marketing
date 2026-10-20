import type { OpenRouterChatSettings } from "@openrouter/ai-sdk-provider";

export const DEFAULT_CHAT_MODEL = "openrouter/free";
const DEFAULT_CHAT_MODEL_KEY = "free-router";
export const MAX_MODEL_OUTPUT_TOKENS = 2048;
const MAX_MODEL_INPUT_BYTES = 500_000;

export interface ChatModelEnv {
  CHAT_OWNER_USER_ID?: string;
  CHAT_PAID_MODELS_ENABLED?: string;
  CHAT_TENANT_MONTHLY_BUDGET_MICRO_USD?: string;
  CHAT_GLOBAL_MONTHLY_BUDGET_MICRO_USD?: string;
}
export interface ChatModelOption {
  key: string;
  label: string;
  description: string;
  free: boolean;
  enabled: boolean;
  disabledReason?: string;
}
export interface ChatModelSettings {
  selectedKey: string;
  model: string;
  models: ChatModelOption[];
  lastModelId?: string;
  limits: {
    globalRequestsPerDay: number;
    tenantRequestsPerDay: number;
    tenantTokensPerDay: number;
  };
}
export interface ChatModelPolicy {
  key: string;
  id: string;
  label: string;
  description: string;
  free: boolean;
  promptMicroUsdPerMillion: number;
  completionMicroUsdPerMillion: number;
  unavailableReason?: string;
}
const MODELS: readonly ChatModelPolicy[] = Object.freeze([
  {
    key: "free-router",
    id: DEFAULT_CHAT_MODEL,
    label: "Automatic · Free",
    description: "OpenRouter chooses an eligible free model with tool support.",
    free: true,
    promptMicroUsdPerMillion: 0,
    completionMicroUsdPerMillion: 0,
  },
  {
    key: "xing-free",
    id: "xingchen-agi/xing4.0-29b-a4b:free",
    label: "Xing 4.0 · Free",
    description:
      "A tool-capable free alternative with a listed zero-data-retention provider. No paid fallback.",
    free: true,
    promptMicroUsdPerMillion: 0,
    completionMicroUsdPerMillion: 0,
  },
  {
    key: "minimax-free",
    id: "minimax/minimax-m3.1-flash:free",
    label: "MiniMax M3.1 Flash · Free",
    description:
      "A free tool-capable alternative. Availability depends on privacy-compatible capacity.",
    free: true,
    promptMicroUsdPerMillion: 0,
    completionMicroUsdPerMillion: 0,
    unavailableReason:
      "No zero-data-retention provider is currently listed for this free model. Choose Automatic or Xing Free.",
  },
  {
    key: "nemotron-free",
    id: "nvidia/nemotron-3.5-lightning:free",
    label: "Nemotron 3.5 Lightning · Free",
    description: "A free tool-capable alternative. No paid fallback.",
    free: true,
    promptMicroUsdPerMillion: 0,
    completionMicroUsdPerMillion: 0,
    unavailableReason:
      "No zero-data-retention provider is currently listed for this free model. Choose Automatic or Xing Free.",
  },
  {
    key: "gpt41-mini",
    id: "openai/gpt-4.1-mini",
    label: "GPT-4.1 Mini · Paid",
    description:
      "Owner-only opt-in. A shared provider cap no higher than the tenant and application caps is required.",
    free: false,
    promptMicroUsdPerMillion: 400_000,
    completionMicroUsdPerMillion: 1_600_000,
  },
]);

export function defaultChatModelKey(env: { OPENROUTER_MODEL?: string }): string {
  const configured = (env.OPENROUTER_MODEL ?? "").trim();
  return (
    MODELS.find((model) => model.free && model.id === configured)?.key ?? DEFAULT_CHAT_MODEL_KEY
  );
}

export type ModelFailure =
  | "model_not_allowed"
  | "paid_not_enabled"
  | "budget"
  | "context"
  | "privacy"
  | "provider";
export class ChatModelError extends Error {
  constructor(readonly code: ModelFailure) {
    super(modelFailureMessage(code));
  }
}
export function modelFailureMessage(code: ModelFailure): string {
  switch (code) {
    case "model_not_allowed":
      return "That model is not available for this account. Choose an allowed model.";
    case "paid_not_enabled":
      return "Paid models are disabled without explicit owner authorization and verified spending limits. Choose a free model.";
    case "budget":
      return "Chat request, token or spending limit reached. Shared free capacity is limited. Wait for the allowance to reset; no automatic paid fallback is used.";
    case "context":
      return "This conversation or enabled tool set is too large for the model budget. Start a shorter conversation or enable fewer tools.";
    case "privacy":
      return "No eligible provider is available under the required privacy policy. Try another free model or retry later; privacy settings are never relaxed.";
    default:
      return "The selected model could not respond. Your conversation is saved. Try another allowed model or retry later. No automatic model switch was made.";
  }
}
/** Native error frames may contain arbitrary upstream text; show only our own notices. */
export function safeModelFailureMessage(error: unknown): string {
  for (const code of [
    "model_not_allowed",
    "paid_not_enabled",
    "budget",
    "context",
    "privacy",
    "provider",
  ] as const) {
    const message = modelFailureMessage(code);
    if (error === message) return message;
  }
  return modelFailureMessage("provider");
}

function currencyCap(value: string | undefined): number {
  if (!value || !/^[1-9]\d{0,9}$/.test(value)) return 0;
  const cap = Number(value);
  return Number.isSafeInteger(cap) && cap <= 1_000_000_000 ? cap : 0;
}
export function paidModelCaps(env: ChatModelEnv): { tenant: number; global: number } {
  return {
    tenant: currencyCap(env.CHAT_TENANT_MONTHLY_BUDGET_MICRO_USD),
    global: currencyCap(env.CHAT_GLOBAL_MONTHLY_BUDGET_MICRO_USD),
  };
}
export function paidProviderCapLimit(env: ChatModelEnv): number {
  const caps = paidModelCaps(env);
  return Math.min(caps.tenant, caps.global);
}

export function paidModelConfigured(env: ChatModelEnv, userId: string): boolean {
  const caps = paidModelCaps(env);
  return (
    !!userId &&
    userId === env.CHAT_OWNER_USER_ID &&
    env.CHAT_PAID_MODELS_ENABLED === "true" &&
    caps.tenant > 0 &&
    caps.global > 0
  );
}
export function availableChatModels(
  env: ChatModelEnv,
  userId: string,
  verifiedProviderCap = false,
): ChatModelOption[] {
  return MODELS.filter((model) => model.free || userId === env.CHAT_OWNER_USER_ID).map((model) => ({
    key: model.key,
    label: model.label,
    description: model.free
      ? model.description
      : model.description +
        (paidModelConfigured(env, userId)
          ? " Tenant cap: $" +
            (paidModelCaps(env).tenant / 1_000_000).toFixed(6) +
            "/UTC month; service cap: $" +
            (paidModelCaps(env).global / 1_000_000).toFixed(6) +
            "/UTC month. Reservations are conservative ceilings, not a bill."
          : ""),
    free: model.free,
    enabled:
      !model.unavailableReason &&
      (model.free || (paidModelConfigured(env, userId) && verifiedProviderCap)),
    ...(model.unavailableReason ? { disabledReason: model.unavailableReason } : {}),
    ...(!model.free && !(paidModelConfigured(env, userId) && verifiedProviderCap)
      ? { disabledReason: modelFailureMessage("paid_not_enabled") }
      : {}),
  }));
}
export function selectChatModel(
  key: unknown,
  env: ChatModelEnv,
  userId: string,
  verifiedProviderCap = false,
): ChatModelPolicy {
  const model =
    typeof key === "string" ? MODELS.find((candidate) => candidate.key === key) : undefined;
  if (!model) throw new ChatModelError("model_not_allowed");
  if (model.unavailableReason) throw new ChatModelError("privacy");
  if (!model.free && !(paidModelConfigured(env, userId) && verifiedProviderCap))
    throw new ChatModelError("paid_not_enabled");
  return model;
}
export function modelProviderSettings(model: ChatModelPolicy): OpenRouterChatSettings {
  return {
    provider: {
      data_collection: "deny",
      zdr: true,
      allow_fallbacks: false,
      require_parameters: true,
      max_price: {
        prompt: model.promptMicroUsdPerMillion / 1_000_000,
        completion: model.completionMicroUsdPerMillion / 1_000_000,
        request: 0,
        image: 0,
        audio: 0,
      },
      ...(!model.free ? { only: ["openai"] } : {}),
    },
  };
}

export interface VerifiedProviderBudget {
  capMicroUsd: number;
  remainingMicroUsd: number;
}
/** Read-only preflight; no key creation, credits, configuration or paid inference. */
export async function verifyPaidProviderBudget(
  apiKey: string,
  globalCap: number,
  fetcher: typeof fetch = fetch,
): Promise<VerifiedProviderBudget | null> {
  if (!apiKey || !Number.isSafeInteger(globalCap) || globalCap < 1) return null;
  try {
    const response = await fetcher("https://openrouter.ai/api/v1/key", {
      method: "GET",
      headers: { Authorization: "Bearer " + apiKey },
      redirect: "manual",
      signal: AbortSignal.timeout(5000),
    });
    if (!response.ok) return null;
    const { data } = (await response.json()) as {
      data?: {
        limit?: unknown;
        limit_remaining?: unknown;
        limit_reset?: unknown;
        include_byok_in_limit?: unknown;
      };
    };
    if (
      !data ||
      typeof data.limit !== "number" ||
      typeof data.limit_remaining !== "number" ||
      !Number.isFinite(data.limit) ||
      !Number.isFinite(data.limit_remaining) ||
      data.limit <= 0 ||
      data.limit_remaining <= 0 ||
      data.include_byok_in_limit !== true ||
      (data.limit_reset != null && data.limit_reset !== "monthly")
    )
      return null;
    const capMicroUsd = Math.ceil(data.limit * 1_000_000);
    const remainingMicroUsd = Math.floor(data.limit_remaining * 1_000_000);
    return Number.isSafeInteger(capMicroUsd) &&
      capMicroUsd <= globalCap &&
      Number.isSafeInteger(remainingMicroUsd) &&
      remainingMicroUsd > 0 &&
      remainingMicroUsd <= capMicroUsd
      ? { capMicroUsd, remainingMicroUsd }
      : null;
  } catch {
    return null;
  }
}

export function modelRequestReservation(
  body: string,
  model: ChatModelPolicy,
): { inputTokens: number; outputTokens: number; costMicroUsd: number; paid: boolean } {
  const bytes = new TextEncoder().encode(body).byteLength;
  if (bytes > MAX_MODEL_INPUT_BYTES) throw new ChatModelError("context");
  let payload: Record<string, unknown>;
  try {
    payload = JSON.parse(body) as Record<string, unknown>;
  } catch {
    throw new ChatModelError("model_not_allowed");
  }
  if (
    !payload ||
    payload.model !== model.id ||
    payload.models !== undefined ||
    payload.plugins !== undefined ||
    payload.web_search_options !== undefined ||
    payload.audio !== undefined ||
    payload.prediction !== undefined ||
    payload.cache_control !== undefined ||
    (payload.modalities !== undefined &&
      (!Array.isArray(payload.modalities) ||
        payload.modalities.some((value) => value !== "text"))) ||
    (!model.free && (payload.reasoning !== undefined || payload.include_reasoning === true))
  )
    throw new ChatModelError("model_not_allowed");
  const tokenBounds = [payload.max_tokens, payload.max_completion_tokens].filter(
    (value) => value !== undefined,
  );
  if (
    !tokenBounds.length ||
    tokenBounds.some(
      (value) =>
        !Number.isSafeInteger(value) ||
        Number(value) < 1 ||
        Number(value) > MAX_MODEL_OUTPUT_TOKENS,
    )
  )
    throw new ChatModelError("context");
  const tokens = Math.max(...tokenBounds.map(Number));
  if (!Array.isArray(payload.messages)) throw new ChatModelError("model_not_allowed");
  for (const raw of payload.messages) {
    if (!raw || typeof raw !== "object") throw new ChatModelError("model_not_allowed");
    const message = raw as Record<string, unknown>;
    if (message.cache_control !== undefined) throw new ChatModelError("model_not_allowed");
    if (Array.isArray(message.content)) {
      for (const rawPart of message.content) {
        const part = rawPart as Record<string, unknown> | null;
        if (
          !part ||
          part.type !== "text" ||
          typeof part.text !== "string" ||
          part.cache_control !== undefined
        )
          throw new ChatModelError("model_not_allowed");
      }
    } else if (message.content != null && typeof message.content !== "string")
      throw new ChatModelError("model_not_allowed");
  }
  const expected = modelProviderSettings(model).provider!;
  const provider = payload.provider as Record<string, unknown> | undefined;
  if (
    !provider ||
    provider.data_collection !== "deny" ||
    provider.zdr !== true ||
    provider.allow_fallbacks !== false ||
    provider.require_parameters !== true
  )
    throw new ChatModelError("privacy");
  const prices = provider.max_price as Record<string, unknown> | undefined;
  for (const [key, value] of Object.entries(expected.max_price!))
    if (
      !prices ||
      (typeof prices[key] !== "number" && typeof prices[key] !== "string") ||
      !Number.isFinite(Number(prices[key])) ||
      Number(prices[key]) !== Number(value)
    )
      throw new ChatModelError("model_not_allowed");
  if (
    !model.free &&
    (!Array.isArray(provider.only) || provider.only.length !== 1 || provider.only[0] !== "openai")
  )
    throw new ChatModelError("model_not_allowed");
  const inputTokens = bytes + 1024; // Bytes plus framing reserve conservatively overestimate text tokenization.
  const outputTokens = Number(tokens);
  const costMicroUsd = Math.ceil(
    (inputTokens * model.promptMicroUsdPerMillion +
      outputTokens * model.completionMicroUsdPerMillion) /
      1_000_000,
  );
  return { inputTokens, outputTokens, costMicroUsd, paid: !model.free };
}
