import { PRODUCT_CSS } from "../product-css.generated.js";
import { renderProductNavigation } from "../product-navigation.js";
import { createRoot } from "react-dom/client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAgent } from "agents/react";
import { useAgentChat } from "@cloudflare/ai-chat/react";
import { useAISDKRuntime } from "@assistant-ui/ai-sdk";
import {
  AssistantRuntimeProvider,
  ThreadPrimitive,
  MessagePrimitive,
  ComposerPrimitive,
  useAuiState,
  type ToolCallMessagePartProps,
} from "@assistant-ui/react";
import { MarkdownTextPrimitive } from "@assistant-ui/react-markdown";
import { ModelSelector, type ChatModelSettings } from "./model-selector.js";
import { safeModelFailureMessage } from "./models.js";
import "./style.css";

type ChatThread = { id: string; title: string; createdAt: string; updatedAt: string };
type ChatSession = {
  thread?: ChatThread;
  model: string;
  conversation: string;
  draftKey: string;
  expiresAt: number;
};
type SessionGate = "loading" | "unauthenticated" | "owner-only" | "unavailable" | "ready";
type ClerkLike = {
  session?: { id: string } | null;
  loaded?: boolean;
  addListener(listener: (payload: { session?: { id?: string } | null }) => void): () => void;
  signOut(options?: { redirectUrl?: string }): void | Promise<void>;
};
type ClerkWindow = typeof window & { Clerk?: ClerkLike };

const SESSION_ENDPOINT = "/api/chat/session";
const DRAFT_PREFIX = "gog:chat:draft:";
const deletedDraftKeys = new Set<string>();
const draftStorageKey = (draftKey: string) => DRAFT_PREFIX + draftKey;
// Clerk is initialized by a script the server injects into the built HTML.
const clerk = (): ClerkLike | null => (window as ClerkWindow).Clerk ?? null;
const activeClerkSessionId = () => {
  const c = clerk();
  return c?.loaded ? (c.session?.id ?? null) : null;
};

function UserMessage() {
  const stopped = useAuiState((s) => s.message.metadata.custom.gogStopped === true);
  return (
    <MessagePrimitive.Root className="message user">
      <span className="speaker">You</span>
      <MessagePrimitive.Parts />
      {stopped && (
        <p className="turn-note" data-turn-state="stopped">
          Response stopped. Any partial reply is kept below.
        </p>
      )}
    </MessagePrimitive.Root>
  );
}

function Markdown() {
  return <MarkdownTextPrimitive />;
}

function GoogleToolCard({
  toolName,
  args,
  result,
  isError,
  approval,
  respondToApproval,
}: ToolCallMessagePartProps) {
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const pending = approval && approval.approved === undefined && !approval.resolution;
  const decide = async (approved: boolean) => {
    setSending(true);
    setError("");
    try {
      await respondToApproval({ approved });
    } catch {
      setError("Could not record your decision. Check your connection and try again.");
    } finally {
      setSending(false);
    }
  };
  const failed =
    isError || (result && typeof result === "object" && "isError" in result && result.isError);
  return (
    <section
      className="google-tool"
      aria-label={pending ? "Approve Google action" : "Google tool activity"}
    >
      <h3>{pending ? "Review before changing Google data" : "Google tool"}</h3>
      <p>
        <strong>{toolName}</strong>
      </p>
      <details open={!!pending}>
        <summary>Account, resource and exact arguments</summary>
        <pre>{JSON.stringify(args, null, 2)}</pre>
      </details>
      {pending ? (
        <>
          <p>
            This changes Google data. Approve only if these exact details are right. Approval
            expires after 10 minutes.
          </p>
          <div className="approval-actions">
            <button type="button" disabled={sending} onClick={() => void decide(true)}>
              Approve Google action
            </button>
            <button type="button" disabled={sending} onClick={() => void decide(false)}>
              Don't run
            </button>
          </div>
        </>
      ) : (
        <p role="status">
          {approval?.approved === false
            ? "Denied — nothing was executed."
            : failed
              ? "Google action failed or was denied."
              : result !== undefined
                ? "Google result received."
                : "Waiting for Google…"}
        </p>
      )}
      {result !== undefined && (
        <details>
          <summary>Tool result</summary>
          <pre>{JSON.stringify(result, null, 2)}</pre>
        </details>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function AssistantMessage() {
  return (
    <MessagePrimitive.Root className="message assistant">
      <span className="speaker">Gog</span>
      <MessagePrimitive.Parts
        components={{ Text: Markdown, tools: { Fallback: GoogleToolCard } }}
      />
    </MessagePrimitive.Root>
  );
}

function Chat({
  session,
  onOpenThreads,
  navigationOpen,
  onRename,
  onDelete,
  onDisconnect,
}: {
  session: ChatSession;
  onOpenThreads: () => void;
  navigationOpen: boolean;
  onRename: () => void;
  onDelete: () => void;
  onDisconnect: () => void;
}) {
  const [connected, setConnected] = useState(false);
  const [hasDraft, setHasDraft] = useState(false);
  const [keyboardFocus, setKeyboardFocus] = useState(false);
  const [draftSaved, setDraftSaved] = useState(true);
  const [announcement, setAnnouncement] = useState("");
  const [turnNotice, setTurnNotice] = useState("");
  const [modelSettings, setModelSettings] = useState<ChatModelSettings | null>(null);
  const [modelSettingsLoading, setModelSettingsLoading] = useState(false);
  const [modelSettingsError, setModelSettingsError] = useState("");
  const [modelBusy, setModelBusy] = useState(false);
  const [modelSettingsVersion, setModelSettingsVersion] = useState(0);
  const draftKey = draftStorageKey(session.draftKey);
  const agent = useAgent({
    agent: "Chat",
    name: session.conversation,
    onOpen: () => setConnected(true),
    onMessage: (event) => {
      try {
        const data = JSON.parse(String(event.data)) as { type?: unknown };
        if (data.type === "gog-model-changed") setModelSettingsVersion((value) => value + 1);
      } catch {
        /* Native protocol frames are handled by the SDK. */
      }
    },
    onClose: () => {
      setConnected(false);
      onDisconnect();
    },
  });
  const transport = useRef<{ readyState: number; reconnect: () => void } | null>(null);
  const lastVerifiedReconnect = useRef(0);
  const verifiedSession = useRef(session);
  transport.current = agent;
  useEffect(() => {
    // A new session object comes only from a successful /api/chat/session check.
    // The SDK deliberately stops after terminal closes; retry through its own transport
    // only after server-side ownership/session verification has succeeded again.
    // Let the SDK own its initial connect; recovery requires a subsequent verified response.
    if (verifiedSession.current === session) return;
    verifiedSession.current = session;
    const socket = transport.current;
    if (!socket) return;
    setConnected(socket.readyState === WebSocket.OPEN);
    if (
      socket.readyState === WebSocket.CLOSED &&
      Date.now() - lastVerifiedReconnect.current >= 3000
    ) {
      lastVerifiedReconnect.current = Date.now();
      socket.reconnect();
    }
  }, [session]);
  const chat = useAgentChat({
    agent,
    onFinish: ({ message, isAbort, isError }) => {
      if (isAbort || isError) return;
      const text = message.parts
        .filter((p) => p.type === "text")
        .map((p) => p.text)
        .join(" ")
        .trim();
      onDisconnect();
      setAnnouncement(
        "Gog replied: " +
          text.slice(0, 1200) +
          (text.length > 1200 ? " Full response is in the conversation." : ""),
      );
    },
    onTurnEnd: (event) => {
      if (event.outcome === "error") {
        setTurnNotice(safeModelFailureMessage(event.error));
        setAnnouncement("");
        return;
      }
      if (event.outcome !== "aborted") return;
      setTurnNotice("Response stopped. Your sent messages and any partial reply are kept.");
      setAnnouncement("");
      // Native turn outcomes supply the exact user-message IDs, including on reconnect.
      // Persist only a presentation annotation in the SDK's existing message store.
      if (event.messageIds?.length)
        void agent.call("markStopped", [event.messageIds]).catch(() => {
          setTurnNotice("Response stopped, but its status could not be saved. Reconnect to retry.");
        });
    },
  });
  const runtime = useAISDKRuntime(chat);
  const running = chat.status === "streaming" || chat.status === "submitted";
  const modelReady =
    !!modelSettings?.models.some(
      (model) => model.key === modelSettings.selectedKey && model.enabled,
    ) && modelSettings.model !== "unavailable";
  useEffect(() => {
    if (!connected) return;
    let active = true;
    setModelSettingsLoading(true);
    setModelSettingsError("");
    agent
      .call("modelSettings")
      .then((settings) => {
        if (!active) return;
        setModelSettings(settings as ChatModelSettings);
      })
      .catch(() => {
        if (!active) return;
        setModelSettings(null);
        setModelSettingsError(
          "Could not load saved model settings. Sending is paused until the connection recovers.",
        );
      })
      .finally(() => {
        if (active) setModelSettingsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [agent, connected, modelSettingsVersion]);
  const selectModel = async (key: string) => {
    if (!modelSettings || modelBusy || running || !connected) return;
    setModelBusy(true);
    setModelSettingsError("");
    try {
      const settings = (await agent.call("selectModel", [key])) as ChatModelSettings;
      setModelSettings(settings);
    } catch {
      setModelSettingsError(
        "Could not save your model choice. Check your connection, then choose it again.",
      );
    } finally {
      setModelBusy(false);
    }
  };
  useEffect(() => {
    const composer = runtime.thread.composer;
    try {
      const saved = window.sessionStorage.getItem(draftKey);
      if (saved && !composer.getState().text) composer.setText(saved);
    } catch {
      setDraftSaved(false);
    }
    const save = () => {
      const text = composer.getState().text;
      setHasDraft(Boolean(text));
      try {
        if (deletedDraftKeys.has(draftKey)) window.sessionStorage.removeItem(draftKey);
        else if (text) window.sessionStorage.setItem(draftKey, text);
        else window.sessionStorage.removeItem(draftKey);
        setDraftSaved(true);
      } catch {
        setDraftSaved(false);
      }
    };
    save();
    const unsubscribe = composer.subscribe(save);
    const warnIfNotSaved = (event: BeforeUnloadEvent) => {
      if (!composer.getState().text) return;
      try {
        if (window.sessionStorage.getItem(draftKey) === composer.getState().text) return;
      } catch {
        /* Warn when browser storage is unavailable. */
      }
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnIfNotSaved);
    return () => {
      save();
      unsubscribe();
      window.removeEventListener("beforeunload", warnIfNotSaved);
    };
  }, [runtime, draftKey]);
  useEffect(() => {
    const keyboard = (event: KeyboardEvent) => {
      if (event.key === "Tab") setKeyboardFocus(true);
    };
    const pointer = () => setKeyboardFocus(false);
    window.addEventListener("keydown", keyboard, true);
    window.addEventListener("pointerdown", pointer, true);
    return () => {
      window.removeEventListener("keydown", keyboard, true);
      window.removeEventListener("pointerdown", pointer, true);
    };
  }, []);
  useEffect(() => {
    if (running) {
      setTurnNotice("");
      setAnnouncement("");
    }
  }, [running]);
  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <div className="app">
        <header className="chat-thread-bar">
          <div className="thread-heading">
            <button
              type="button"
              className="nav-toggle quiet"
              aria-label="Open conversations"
              aria-expanded={navigationOpen}
              aria-controls="chat-sidebar"
              onClick={onOpenThreads}
            >
              ☰
            </button>
            <h1>{session.thread?.title ?? "Original conversation"}</h1>
          </div>
          <div className="thread-actions">
            <button type="button" className="quiet" onClick={onRename}>
              Rename
            </button>
            <button type="button" className="quiet" onClick={onDelete}>
              Delete
            </button>
          </div>
        </header>
        <div className="notice" role="note">
          Google tools use your enabled permissions. Writes need your approval. Open{" "}
          <a href="/#google-data">Google settings</a> or return to <a href="/">Home</a>.
        </div>
        <main aria-label="Chat">
          <ThreadPrimitive.Root className="thread">
            <ThreadPrimitive.Viewport
              className="viewport"
              role="region"
              aria-label="Conversation"
              tabIndex={0}
              autoScroll={chat.messages.length > 0}
              scrollToBottomOnInitialize={chat.messages.length > 0}
              scrollToBottomOnThreadSwitch={chat.messages.length > 0}
            >
              <ThreadPrimitive.Empty>
                <section className="empty">
                  <span className="eyebrow">YOUR NEXT CAMPAIGN</span>
                  <h2>What needs moving forward?</h2>
                  <p>
                    Draft a brief, work through results, or prepare a client handoff.
                    <br />
                    Your sent messages are saved as you go.
                  </p>
                  <div className="suggestions">
                    {[
                      "Help me draft a campaign brief",
                      "Summarise my campaign results",
                      "Outline a client presentation",
                    ].map((prompt) => (
                      <ThreadPrimitive.Suggestion
                        key={prompt}
                        prompt={prompt}
                        send={modelReady && connected && !modelBusy && !modelSettingsLoading}
                      >
                        {prompt}
                        <span aria-hidden="true">↗</span>
                      </ThreadPrimitive.Suggestion>
                    ))}
                  </div>
                </section>
              </ThreadPrimitive.Empty>
              <ThreadPrimitive.Messages components={{ UserMessage, AssistantMessage }} />
            </ThreadPrimitive.Viewport>
            <div className="composer-area">
              <div role="status" className="status">
                {!connected
                  ? "Reconnecting… Your draft is kept in this tab."
                  : running
                    ? "Gog is responding…"
                    : turnNotice || "Connected · Sent messages saved"}
              </div>
              <div
                className="sr-only"
                role="status"
                aria-live="polite"
                aria-atomic="true"
                aria-label="Latest assistant reply"
              >
                {announcement}
              </div>
              {chat.error && (
                <p role="alert" className="error">
                  {chat.error.message || "The response failed. Please try again."}
                </p>
              )}
              <ComposerPrimitive.Root className="composer" data-keyboard-focus={keyboardFocus}>
                <ComposerPrimitive.Input
                  aria-label="Message"
                  placeholder="Ask about a campaign, brief or report…"
                  disabled={!connected}
                  maxLength={8000}
                  rows={2}
                />
                {running ? (
                  <ComposerPrimitive.Cancel className="send" aria-label="Stop response">
                    Stop ■
                  </ComposerPrimitive.Cancel>
                ) : (
                  <ComposerPrimitive.Send
                    className="send"
                    aria-label="Send message"
                    disabled={!connected || !modelReady || modelBusy || modelSettingsLoading}
                  >
                    Send ↑
                  </ComposerPrimitive.Send>
                )}
              </ComposerPrimitive.Root>
              {hasDraft && !draftSaved && (
                <p className="draft-note" role="alert">
                  Draft could not be saved. Copy it before leaving this page.
                </p>
              )}
              <div className="footnote">
                <span>AI can make mistakes. Check important details.</span>
              </div>
              <ModelSelector
                settings={modelSettings}
                connected={connected}
                running={running}
                busy={modelBusy}
                loading={modelSettingsLoading}
                error={modelSettingsError}
                onSelect={selectModel}
                onRetry={() => setModelSettingsVersion((value) => value + 1)}
              />
            </div>
          </ThreadPrimitive.Root>
        </main>
        <footer>
          Google access follows your saved permissions. Review every write before approving.
        </footer>
      </div>
    </AssistantRuntimeProvider>
  );
}

function App() {
  const authGeneration = useRef(0);
  const clerkSession = useRef<string | null | undefined>(undefined);
  const [session, setSession] = useState<ChatSession | null>(null);
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [pendingDeletions, setPendingDeletions] = useState<{ id: string; draftKey: string }[]>([]);
  const [threadError, setThreadError] = useState("");
  const [threadBusy, setThreadBusy] = useState(false);
  const [navigationOpen, setNavigationOpen] = useState(false);
  const [mobile, setMobile] = useState(() => window.matchMedia("(max-width: 900px)").matches);
  const [threadAction, setThreadAction] = useState<"rename" | "delete" | null>(null);
  const [threadName, setThreadName] = useState("");
  const dialog = useRef<HTMLDialogElement>(null);
  const sidebar = useRef<HTMLElement>(null);
  const selectedThread = () => new URL(window.location.href).searchParams.get("thread");
  const sessionEndpoint = () =>
    SESSION_ENDPOINT + (selectedThread() ? "?thread=" + encodeURIComponent(selectedThread()!) : "");
  const refreshThreads = useCallback(async () => {
    const generation = authGeneration.current;
    const identity = activeClerkSessionId();
    try {
      const r = await fetch("/api/chat/threads", { signal: AbortSignal.timeout(10000) });
      if (!r.ok) throw new Error("Thread list unavailable");
      const data = (await r.json()) as {
        threads: ChatThread[];
        pendingDeletions?: { id: string; draftKey: string }[];
      };
      if (generation === authGeneration.current && identity === activeClerkSessionId()) {
        setThreads(data.threads);
        setPendingDeletions(data.pendingDeletions ?? []);
        for (const pending of data.pendingDeletions ?? []) {
          deletedDraftKeys.add(draftStorageKey(pending.draftKey));
          try {
            sessionStorage.removeItem(draftStorageKey(pending.draftKey));
          } catch {
            /* Storage can be disabled. */
          }
        }
        setThreadError((current) =>
          current.startsWith("Could not load conversations") ? "" : current,
        );
      }
    } catch {
      if (generation === authGeneration.current)
        setThreadError(
          "Could not load conversations. Your saved chats are kept. Try refreshing the list.",
        );
    }
  }, []);
  const [gate, setGate] = useState<SessionGate>("loading");
  const [message, setMessage] = useState("");
  const [signOutFailed, setSignOutFailed] = useState(false);
  const applySession = useCallback(
    (data: ChatSession) => {
      // A recovered/new thread must not inherit a dialog whose DOM was unmounted.
      setThreadAction(null);
      setNavigationOpen(false);
      setSession(data);
      setGate("ready");
      setMessage("");
      if (data.thread)
        window.history.replaceState(null, "", "/chat?thread=" + encodeURIComponent(data.thread.id));
      void refreshThreads();
    },
    [refreshThreads],
  );
  const signOutView = useCallback((notice = "") => {
    authGeneration.current++;
    setSession(null);
    setThreads([]);
    setPendingDeletions([]);
    setThreadError("");
    setThreadBusy(false);
    setThreadAction(null);
    setNavigationOpen(false);
    setGate("unauthenticated");
    setMessage(notice);
  }, []);
  const ownerOnlyView = useCallback((notice = "") => {
    authGeneration.current++;
    setSession(null);
    setThreads([]);
    setPendingDeletions([]);
    setThreadError("");
    setThreadBusy(false);
    setThreadAction(null);
    setNavigationOpen(false);
    setGate("owner-only");
    setMessage(notice);
  }, []);
  const loadSession = useCallback(async () => {
    const generation = authGeneration.current;
    const identity = activeClerkSessionId();
    if (!identity) {
      signOutView();
      return;
    }
    setGate("loading");
    setMessage("");
    try {
      let r = await fetch(sessionEndpoint(), { signal: AbortSignal.timeout(10000) });
      if (generation !== authGeneration.current || activeClerkSessionId() !== identity) return;
      if (r.status === 404 && selectedThread()) {
        // A deleted/stale bookmark must not strand the entire workspace.
        window.history.replaceState(null, "", "/chat");
        r = await fetch(SESSION_ENDPOINT, { signal: AbortSignal.timeout(10000) });
      }
      if (generation !== authGeneration.current || activeClerkSessionId() !== identity) return;
      if (r.ok) {
        const data = (await r.json()) as ChatSession;
        if (generation === authGeneration.current && activeClerkSessionId() === identity)
          applySession(data);
      } else if (r.status === 401) signOutView();
      else if (r.status === 403) ownerOnlyView();
      else throw new Error("Session check failed");
    } catch {
      if (generation !== authGeneration.current || activeClerkSessionId() !== identity) return;
      setSession(null);
      setGate("unavailable");
      setMessage(
        "Could not reach your workspace. Check your connection, then retry. Your draft is kept in this tab.",
      );
    }
  }, [applySession, signOutView, ownerOnlyView]);
  // Session recheck while the conversation is open. Transient failures are quiet:
  // the native socket reconnects on its own and the draft stays in this tab.
  const recheck = useCallback(async () => {
    const generation = authGeneration.current;
    const identity = activeClerkSessionId();
    if (!identity) {
      signOutView();
      return;
    }
    try {
      const r = await fetch(sessionEndpoint(), { signal: AbortSignal.timeout(10000) });
      if (generation !== authGeneration.current || activeClerkSessionId() !== identity) return;
      if (r.ok) {
        const data = (await r.json()) as ChatSession;
        if (generation !== authGeneration.current || activeClerkSessionId() !== identity) return;
        setSession((current) => {
          if (
            generation !== authGeneration.current ||
            activeClerkSessionId() !== identity ||
            !current
          )
            return current;
          // Same conversation: refresh fields in place, keeping the socket mounted.
          if (current.conversation === data.conversation && current.draftKey === data.draftKey)
            return {
              ...current,
              model: data.model,
              expiresAt: data.expiresAt,
              thread: data.thread,
            };
          return data;
        });
        setMessage("");
        void refreshThreads();
      } else if (r.status === 404) {
        authGeneration.current++;
        window.history.replaceState(null, "", "/chat");
        setSession(null);
        void loadSession();
      } else if (r.status === 401) {
        signOutView(
          "Your session expired or was signed out. Sign in again to continue your saved conversation and draft.",
        );
      } else if (r.status === 403) {
        ownerOnlyView("This workspace is available only to the workspace owner.");
      }
    } catch {
      /* Keep the open conversation; the socket reconnects independently. */
    }
  }, [signOutView, ownerOnlyView, loadSession, refreshThreads]);
  const signOut = useCallback(async () => {
    // Close the socket first; do not claim the server session was revoked if Clerk fails.
    signOutView();
    setSignOutFailed(false);
    try {
      const c = clerk();
      if (!c?.signOut) throw new Error("Sign-out is not available");
      await c.signOut({ redirectUrl: "/" });
    } catch {
      setSignOutFailed(true);
      setMessage("Could not complete sign-out. Please try again. Your draft is kept in this tab.");
    }
  }, [signOutView]);
  // Clerk session changes (sign-out elsewhere, session expiry) close the live socket
  // by dropping the mounted conversation until the user signs back in.
  useEffect(() => {
    let unlisten: (() => void) | undefined;
    let attempts = 0;
    const attach = window.setInterval(() => {
      const c = clerk();
      if (c?.loaded && !unlisten) {
        unlisten = c.addListener((payload) => {
          const next = payload.session?.id ?? null;
          const previous = clerkSession.current;
          clerkSession.current = next;
          if (!next) {
            signOutView(
              "Your session expired or was signed out. Sign in again to continue your saved conversation and draft.",
            );
          } else if (previous !== next) {
            // Drop the prior identity before re-verifying; stale metadata cannot revive its draft.
            signOutView();
            void loadSession();
          }
        });
      }
      if (c?.loaded || ++attempts > 20) {
        window.clearInterval(attach);
        if (!c?.loaded) {
          setGate("unavailable");
          setMessage(
            "Could not load sign-in. Check your connection, then reload this page. Your draft is kept in this tab.",
          );
        }
      }
    }, 500);
    return () => {
      window.clearInterval(attach);
      unlisten?.();
    };
  }, [signOutView, loadSession]);
  // Clerk's real session expiry is authoritative: when it lapses, unmount the
  // conversation (closing the socket) and re-check before remounting.
  useEffect(() => {
    if (!session) return;
    const timer = window.setTimeout(
      () => {
        setGate("loading");
        void loadSession();
      },
      Math.min(2147483647, Math.max(0, session.expiresAt - Date.now())),
    );
    return () => window.clearTimeout(timer);
  }, [session, loadSession]);
  useEffect(() => {
    if (!session) return;
    const interval = window.setInterval(() => void recheck(), 30000);
    const onVisible = () => {
      if (document.visibilityState === "visible") void recheck();
    };
    const onFocus = () => void recheck();
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", onFocus);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", onFocus);
    };
  }, [session, recheck]);
  const openThread = useCallback(
    (id?: string, replace = false) => {
      authGeneration.current++;
      setThreadBusy(false);
      const url = id ? "/chat?thread=" + encodeURIComponent(id) : "/chat";
      if (replace) window.history.replaceState(null, "", url);
      else window.history.pushState(null, "", url);
      setNavigationOpen(false);
      setSession(null);
      setThreadAction(null);
      void loadSession();
    },
    [loadSession],
  );
  useEffect(() => {
    const onPop = () =>
      openThread(new URL(window.location.href).searchParams.get("thread") ?? undefined, true);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, [openThread]);
  useEffect(() => {
    const media = window.matchMedia("(max-width: 900px)");
    const update = () => setMobile(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  useEffect(() => {
    if (!navigationOpen || !mobile) return;
    sidebar.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setNavigationOpen(false);
    };
    window.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("keydown", escape);
      document.querySelector<HTMLButtonElement>("[aria-label='Open conversations']")?.focus();
    };
  }, [navigationOpen, mobile]);
  useEffect(() => {
    if (threadAction && dialog.current && !dialog.current.open) dialog.current.showModal();
    if (!threadAction && dialog.current?.open) dialog.current.close();
  }, [threadAction]);
  const mutateThread = async (action: "new" | "rename" | "delete") => {
    if (threadBusy) return;
    const generation = authGeneration.current;
    const identity = activeClerkSessionId();
    const id = session?.thread?.id ?? "original";
    setThreadBusy(true);
    setThreadError("");
    // Do not let typing/sending land in the previous thread while creation is pending.
    if (action === "new") {
      setSession(null);
      setGate("loading");
    }
    try {
      const r = await fetch(
        "/api/chat/threads" + (action === "new" ? "" : "/" + encodeURIComponent(id)),
        {
          method: action === "new" ? "POST" : action === "rename" ? "PATCH" : "DELETE",
          headers: { "Content-Type": "application/json" },
          ...(action !== "delete"
            ? { body: JSON.stringify({ title: action === "new" ? "New chat" : threadName.trim() }) }
            : {}),
          signal: AbortSignal.timeout(30000),
        },
      );
      if (!r.ok || (action === "delete" && r.status !== 204))
        throw new Error("Conversation action failed");
      // Native destruction can close the socket and recover to another thread
      // before this HTTP acknowledgement arrives. Clear ONLY the captured,
      // successfully deleted thread's draft even when that recovery changed the
      // generation; stale responses must still never remount/navigate the UI.
      if (action === "delete" && session) {
        deletedDraftKeys.add(draftStorageKey(session.draftKey));
        try {
          sessionStorage.removeItem(draftStorageKey(session.draftKey));
        } catch {
          /* Storage can be disabled. */
        }
      }
      if (generation !== authGeneration.current || identity !== activeClerkSessionId()) return;
      if (action === "delete") {
        openThread(undefined, true);
      } else {
        const data = (await r.json()) as { thread: ChatThread };
        if (generation !== authGeneration.current || identity !== activeClerkSessionId()) return;
        if (action === "new") openThread(data.thread.id);
        else {
          setSession((current) => (current ? { ...current, thread: data.thread } : current));
          setThreadAction(null);
          void refreshThreads();
        }
      }
    } catch {
      if (generation === authGeneration.current && identity === activeClerkSessionId()) {
        if (action === "new") {
          setSession(session);
          setGate("ready");
        }
        void refreshThreads();
        setThreadError(
          "Could not " +
            (action === "new" ? "create" : action) +
            " the conversation. Refresh the list before retrying.",
        );
      }
    } finally {
      if (generation === authGeneration.current && identity === activeClerkSessionId())
        setThreadBusy(false);
    }
  };
  const retryDeletion = async (pending: { id: string; draftKey: string }) => {
    if (threadBusy) return;
    const generation = authGeneration.current;
    const identity = activeClerkSessionId();
    setThreadBusy(true);
    try {
      const response = await fetch("/api/chat/threads/" + encodeURIComponent(pending.id), {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(30000),
      });
      if (response.status !== 204) throw new Error("Cleanup pending");
      if (generation !== authGeneration.current || identity !== activeClerkSessionId()) return;
      deletedDraftKeys.add(draftStorageKey(pending.draftKey));
      try {
        sessionStorage.removeItem(draftStorageKey(pending.draftKey));
      } catch {
        /* Storage can be disabled. */
      }
      setThreadError("");
      await refreshThreads();
      if (session?.thread?.id === pending.id) openThread(undefined, true);
    } catch {
      if (generation === authGeneration.current)
        setThreadError(
          "Deletion cleanup is still pending. Your other conversations are available; retry when the connection recovers.",
        );
    } finally {
      if (generation === authGeneration.current && identity === activeClerkSessionId())
        setThreadBusy(false);
    }
  };
  if (gate === "loading")
    return (
      <div className="login">
        <p role="status">Opening your workspace…</p>
      </div>
    );
  if (gate === "unavailable")
    return (
      <main className="login">
        <section className="login-panel">
          <div className="brand">
            gog-marketing<span> / private assistant</span>
          </div>
          <h1>Your workspace is temporarily unavailable</h1>
          <p role="alert">{message}</p>
          <button type="button" onClick={() => window.location.reload()}>
            Retry connection
          </button>
        </section>
      </main>
    );
  if (gate === "owner-only")
    return (
      <main className="login">
        <section className="login-panel">
          <div className="brand">
            gog-marketing<span> / private assistant</span>
          </div>
          <span className="eyebrow">INVITATION REQUIRED</span>
          <h1>This account is not invited yet</h1>
          <p role="alert">
            {message ||
              "Chat is available only to invited accounts. Ask your workspace administrator for access; signing in alone does not enable private features."}
          </p>
          <a className="signin-link" href="/request-access">
            About beta access
          </a>
          <a className="quiet" href="/sign-in?switch=1&returnTo=%2Fchat">
            Use another account
          </a>
        </section>
      </main>
    );
  if (!session)
    return (
      <main className="login">
        <section className="login-panel">
          <div className="brand">
            gog-marketing<span> / private assistant</span>
          </div>
          <span className="eyebrow">A QUIET SPACE FOR YOUR IDEAS</span>
          <h1>
            Your assistant.
            <br />
            Your conversation.
          </h1>
          <p role={signOutFailed ? "alert" : undefined}>
            {message ||
              "A private workspace for invited users. Sign in to pick up where you left off. Your unsent draft stays in this tab."}
          </p>
          {signOutFailed && (
            <button type="button" onClick={() => void signOut()}>
              Retry sign out
            </button>
          )}
          <a
            className="signin-link"
            href={
              selectedThread() ? "/chat?thread=" + encodeURIComponent(selectedThread()!) : "/chat"
            }
          >
            Sign in to continue
          </a>
          <small>Access is restricted. Provider credentials stay on the server.</small>
        </section>
      </main>
    );
  return (
    <>
      <style>{PRODUCT_CSS}</style>
      <div className={"app-shell chat-shell" + (navigationOpen ? " nav-open" : "")}>
        <aside
          className="sidebar"
          id="chat-sidebar"
          ref={sidebar}
          inert={mobile && !navigationOpen}
          aria-label="Application navigation and conversations"
          onKeyDown={(event) => {
            if (!mobile || !navigationOpen || event.key !== "Tab") return;
            const elements = Array.from(
              sidebar.current?.querySelectorAll<HTMLElement>("a,button:not(:disabled)") ?? [],
            );
            const first = elements[0],
              last = elements.at(-1);
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault();
              last?.focus();
            }
            if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault();
              first?.focus();
            }
          }}
        >
          <button
            className="drawer-close quiet"
            type="button"
            aria-label="Close conversations"
            onClick={() => setNavigationOpen(false)}
          >
            Close ×
          </button>
          <a className="sidebar-brand" href="/">
            <span className="brand-mark">g</span>
            <span>gog-marketing</span>
          </a>
          <div className="sidebar-rule" />
          <nav
            className="side-nav"
            aria-label="Application navigation"
            dangerouslySetInnerHTML={{ __html: renderProductNavigation("chat") }}
          />
          <section className="thread-list" aria-label="Saved conversations">
            <div className="thread-list-heading">
              <h2>Conversations</h2>
              <button type="button" onClick={() => void mutateThread("new")} disabled={threadBusy}>
                New chat
              </button>
            </div>
            <ul>
              {threads.map((thread) => (
                <li key={thread.id}>
                  <a
                    href={"/chat?thread=" + encodeURIComponent(thread.id)}
                    aria-current={session.thread?.id === thread.id ? "page" : undefined}
                    onClick={(event) => {
                      if (
                        event.button !== 0 ||
                        event.metaKey ||
                        event.ctrlKey ||
                        event.shiftKey ||
                        event.altKey
                      )
                        return;
                      event.preventDefault();
                      openThread(thread.id);
                    }}
                    title={thread.title}
                  >
                    {thread.title}
                  </a>
                </li>
              ))}
            </ul>
            {pendingDeletions.length > 0 && (
              <section className="pending-deletions" aria-label="Pending conversation cleanup">
                <h3>Deletion cleanup pending</h3>
                <p>
                  These conversations are inaccessible. Finish removing their stored data. More
                  pending items appear as cleanup completes.
                </p>
                {pendingDeletions.map((pending) => (
                  <button
                    key={pending.id}
                    type="button"
                    disabled={threadBusy}
                    onClick={() => void retryDeletion(pending)}
                  >
                    Finish deletion <span className="sr-only">{pending.id}</span>
                  </button>
                ))}
              </section>
            )}
            <button type="button" className="quiet" onClick={() => void refreshThreads()}>
              Refresh conversations
            </button>
          </section>
        </aside>
        {navigationOpen && mobile && (
          <button
            className="nav-overlay"
            type="button"
            aria-label="Close navigation"
            onClick={() => setNavigationOpen(false)}
          />
        )}
        <div className="app-frame" inert={navigationOpen && mobile}>
          <div className="chat-account-bar">
            <span>Private workspace</span>
            <button type="button" className="quiet" onClick={() => void signOut()}>
              Sign out
            </button>
          </div>
          {threadError && (
            <p role="alert" className="thread-error">
              {threadError}
            </p>
          )}
          <Chat
            key={session.conversation}
            session={session}
            navigationOpen={navigationOpen}
            onOpenThreads={() => setNavigationOpen(true)}
            onRename={() => {
              setThreadName(session.thread?.title ?? "Original conversation");
              setThreadAction("rename");
            }}
            onDelete={() => setThreadAction("delete")}
            onDisconnect={recheck}
          />
        </div>
        <nav
          className="bottom-nav"
          inert={navigationOpen && mobile}
          aria-label="Mobile navigation"
          dangerouslySetInnerHTML={{ __html: renderProductNavigation("chat") }}
        />
        <dialog
          ref={dialog}
          aria-labelledby="thread-action-heading"
          onCancel={(event) => {
            if (threadBusy) event.preventDefault();
            else setThreadAction(null);
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (threadAction) void mutateThread(threadAction);
            }}
          >
            <h2 id="thread-action-heading">
              {threadAction === "rename" ? "Rename conversation" : "Delete conversation?"}
            </h2>
            {threadAction === "rename" ? (
              <label>
                Conversation name
                <input
                  value={threadName}
                  onChange={(event) => setThreadName(event.target.value)}
                  maxLength={100}
                  required
                />
              </label>
            ) : (
              <p>
                This permanently removes this conversation, saved replies and pending approvals.
                Other conversations are kept.
              </p>
            )}
            {threadError && <p role="alert">{threadError}</p>}
            <div className="approval-actions">
              <button
                type="submit"
                disabled={threadBusy || (threadAction === "rename" && !threadName.trim())}
              >
                {threadBusy
                  ? "Saving…"
                  : threadAction === "rename"
                    ? "Save name"
                    : "Delete conversation"}
              </button>
              <button type="button" disabled={threadBusy} onClick={() => setThreadAction(null)}>
                Cancel
              </button>
            </div>
          </form>
        </dialog>
      </div>
    </>
  );
}

createRoot(document.getElementById("root")!).render(<App />);
