/**
 * Chat Durable Object.
 *
 * Extends the native @cloudflare/ai-chat AIChatAgent so persistence,
 * replay/resume and cancellation keep working, and layers strictly owner-gated
 * access checks on top:
 * - only the configured CHAT_OWNER_USER_ID may connect or send a frame;
 * - every Clerk UI message frame and chat inference invocation re-validates the
 *   live Clerk session, the active internal tenant, and connection state;
 * - inference budget/text limits live in ctx.storage so reconnect cannot reset
 *   them.
 *
 * Native /get-messages is additionally gated by the Worker routes. No raw
 * provider exception is ever surfaced; inference errors use a fixed message.
 */
import { AIChatAgent } from "@cloudflare/ai-chat";
import {
  convertToModelMessages,
  createUIMessageStream,
  createUIMessageStreamResponse,
  streamText,
  type GenerateTextOnFinishCallback,
  type ToolSet,
} from "ai";
import {
  callable,
  getCurrentAgent,
  type Connection,
  type ConnectionContext,
  type AgentContext,
} from "agents";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import {
  authenticateChatOwner,
  chatConversationDigest,
  chatModelIdentifier,
  verifyActiveClerkSession,
} from "./access.js";
import { HostedRepository } from "../../../state/src/repository.js";
import type { ChatEnv } from "./access.js";
import type { OnChatMessageOptions } from "@cloudflare/ai-chat";

/** Trusted connection state shape used by the native message handler. */
interface ChatConnectionState {
  sessionId: string;
  userId: string;
  tenantId: string;
  expiresAt: number;
}

const MODEL_ERROR =
  "OpenRouter could not respond. Check the server API key, credits and model availability, then try again.";
const UNAUTHORIZED_MESSAGE = "Not authorized.";
const UNAVAILABLE_MESSAGE = "Chat is not available.";
const BUDGET_MESSAGE = "Message limit reached. Wait before trying again (10/minute, 100/day).";
const TEXT_ONLY_MESSAGE =
  "This chat accepts text only, with a maximum conversation size of 100 KB and 8000 characters per message.";
const SYSTEM_PROMPT =
  "You are a helpful, concise private assistant. No marketing tools are connected. Never claim to have accessed accounts or performed external actions.";

const MAX_INFERENCE_TOKENS = 2048;
const INFERENCE_TIMEOUT_MS = 90_000;
const INFERENCE_MAX_RETRIES = 1;
const BUDGET_PER_MINUTE = 10;
const BUDGET_PER_DAY = 100;
const MAX_CONVERSATION_BYTES = 100_000;
const MAX_USER_MESSAGE_CHARS = 8_000;

function chatError(message: string) {
  return createUIMessageStreamResponse({
    stream: createUIMessageStream({
      execute: ({ writer }) => {
        writer.write({ type: "error", errorText: message });
      },
    }),
  });
}

interface PartLike {
  type?: string;
  text?: string;
}

function userMessageTextPrefix(parts: unknown): number {
  if (!Array.isArray(parts)) return 0;
  let count = 0;
  for (const part of parts as PartLike[]) {
    if (part?.type === "text" && typeof part.text === "string") count += part.text.length;
  }
  return count;
}

function userMessageOnlyText(parts: unknown): boolean {
  if (!Array.isArray(parts)) return false;
  return (parts as PartLike[]).every((part) => part?.type === "text");
}

export class Chat extends AIChatAgent<ChatEnv> {
  constructor(ctx: AgentContext, env: ChatEnv) {
    super(ctx, env);
    // Intercept every text frame (including SDK-intercepted chat frames) after
    // the native handler is installed so the guard runs before inference.
    const nativeMessage = this.onMessage.bind(this);
    this.onMessage = async (connection: Connection, message: unknown) => {
      const state = (connection.state as ChatConnectionState | null) ?? null;
      if (
        !state ||
        !state.expiresAt ||
        state.expiresAt <= Date.now() ||
        !state.sessionId ||
        !state.userId ||
        !state.tenantId ||
        !(await this.isAuthorizedConnection(connection))
      ) {
        connection.close(4401, "Sign in again");
        return;
      }
      if (typeof message !== "string" || message.length > MAX_CONVERSATION_BYTES) {
        connection.close(4400, "Message too large");
        return;
      }
      return nativeMessage(connection, message);
    };
  }

  /**
   * Live trust helper: owner match, future expiry, active internal tenant, and
   * a live Clerk session verified through the Backend API. Fails closed.
   */
  async isAuthorizedConnection(connection: Connection): Promise<boolean> {
    const owner = (this.env.CHAT_OWNER_USER_ID ?? "").trim();
    const state = (connection.state as ChatConnectionState | null) ?? null;
    if (
      !owner ||
      !state ||
      state.userId !== owner ||
      !state.sessionId ||
      !state.tenantId ||
      !state.expiresAt ||
      state.expiresAt <= Date.now()
    ) {
      return false;
    }
    try {
      const tenant = await new HostedRepository(this.env.DB).getTenant(state.tenantId);
      if (!tenant || tenant.status !== "active") return false;
      const session = await verifyActiveClerkSession(this.env, state.sessionId, state.userId);
      return session.kind === "active";
    } catch {
      return false;
    }
  }

  /**
   * Authenticates the *full* incoming request through authenticateChatOwner,
   * derives the server-side conversation digest, and confirms the DO instance
   * name matches it. Connection state only ever carries sessionId/userId/
   * tenantId/expiry — never JWTs or tokens.
   */
  async onConnect(connection: Connection, context: ConnectionContext): Promise<void> {
    const outcome = await authenticateChatOwner(
      context.request,
      this.env,
      new HostedRepository(this.env.DB),
    );
    if (outcome.kind === "unauthenticated" || outcome.kind === "handshake") {
      connection.close(4401, "Sign in again");
      return;
    }
    if (outcome.kind === "error") {
      connection.close(outcome.status === 403 ? 4403 : 4401, "Not authorized");
      return;
    }
    const digest = await chatConversationDigest(outcome.session.tenant.id);
    if (this.name !== digest) {
      connection.close(4403, "Not authorized");
      return;
    }
    if (outcome.session.expiresAt <= Date.now()) {
      connection.close(4401, "Sign in again");
      return;
    }
    connection.setState({
      sessionId: outcome.session.sessionId,
      userId: outcome.session.userId,
      tenantId: outcome.session.tenant.id,
      expiresAt: outcome.session.expiresAt,
    });
  }

  async onChatMessage(
    onFinish: GenerateTextOnFinishCallback<ToolSet>,
    options?: OnChatMessageOptions,
  ) {
    // Every inference invocation must come from the currently-authorized
    // connection, with the live Clerk session + active tenant re-verified.
    const invocation = getCurrentAgent<Chat>();
    if (!invocation || !invocation.connection) {
      return chatError(UNAVAILABLE_MESSAGE);
    }
    if (!(await this.isAuthorizedConnection(invocation.connection))) {
      return chatError(UNAUTHORIZED_MESSAGE);
    }
    // Persist the budget in the same owner Durable Object so reconnects cannot reset it.
    const now = Date.now();
    const minute = Math.floor(now / 60_000);
    const day = Math.floor(now / 86_400_000);
    const budget = await this.ctx.storage.get<{
      minute: number;
      minuteCount: number;
      day: number;
      dayCount: number;
    }>("inference-budget");
    const minuteCount = budget?.minute === minute ? (budget.minuteCount ?? 0) : 0;
    const dayCount = budget?.day === day ? (budget.dayCount ?? 0) : 0;
    if (minuteCount >= BUDGET_PER_MINUTE || dayCount >= BUDGET_PER_DAY)
      return chatError(BUDGET_MESSAGE);
    if (JSON.stringify(this.messages).length > MAX_CONVERSATION_BYTES)
      return chatError(TEXT_ONLY_MESSAGE);
    for (const message of this.messages) {
      if (message.role !== "user") continue;
      if (!userMessageOnlyText((message as unknown as { parts?: unknown }).parts)) {
        return chatError(TEXT_ONLY_MESSAGE);
      }
      if (
        userMessageTextPrefix((message as unknown as { parts?: unknown }).parts) >
        MAX_USER_MESSAGE_CHARS
      ) {
        return chatError(TEXT_ONLY_MESSAGE);
      }
    }
    await this.ctx.storage.put("inference-budget", {
      minute,
      minuteCount: minuteCount + 1,
      day,
      dayCount: dayCount + 1,
    });
    if (!this.env.OPENROUTER_API_KEY) return chatError(MODEL_ERROR);
    const provider = createOpenRouter({ apiKey: this.env.OPENROUTER_API_KEY });
    try {
      const result = streamText({
        model: provider(chatModelIdentifier(this.env)),
        system: SYSTEM_PROMPT,
        messages: await convertToModelMessages(this.messages),
        maxOutputTokens: MAX_INFERENCE_TOKENS,
        abortSignal: options?.abortSignal
          ? AbortSignal.any([options.abortSignal, AbortSignal.timeout(INFERENCE_TIMEOUT_MS)])
          : AbortSignal.timeout(INFERENCE_TIMEOUT_MS),
        maxRetries: INFERENCE_MAX_RETRIES,
        onFinish,
        // Never log provider exception objects: they may contain request headers.
        onError: () => {},
      });
      return result.toUIMessageStreamResponse({ onError: () => MODEL_ERROR });
    } catch {
      return chatError(MODEL_ERROR);
    }
  }

  async markStopped(messageIds: string[]): Promise<void> {
    if (
      !Array.isArray(messageIds) ||
      messageIds.length > 20 ||
      messageIds.some((id) => typeof id !== "string" || id.length > 128)
    ) {
      throw new Error("Invalid message IDs.");
    }
    const ids = new Set(messageIds);
    const updates = this.messages
      .filter(
        (message) => message.role === "user" && ids.has((message as { id?: string }).id ?? ""),
      )
      .map((message) => {
        const metadata =
          (message as { metadata?: unknown }).metadata &&
          typeof (message as { metadata?: unknown }).metadata === "object"
            ? (message as { metadata?: unknown }).metadata
            : {};
        return { ...message, metadata: { ...(metadata as object), gogStopped: true } };
      });
    // Native persistMessages merges these annotations into the existing
    // conversation. Unlike saveMessages, it does not generate another model response.
    if (updates.length) await this.persistMessages(updates);
  }
}

// Register the native SDK callable on the method value. Vite 8 leaves TC39
// decorator syntax untransformed; this is the same annotation without that syntax.
callable()(Chat.prototype.markStopped, {
  kind: "method",
  name: "markStopped",
  static: false,
  private: false,
  access: {
    has: (object) => typeof object === "object" && object !== null && "markStopped" in object,
    get: (object) => (object as Chat).markStopped,
  },
  addInitializer: () => {},
  metadata: {},
});
