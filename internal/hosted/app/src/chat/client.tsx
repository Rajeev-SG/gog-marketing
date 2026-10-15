import { createRoot } from "react-dom/client";
import { useCallback, useEffect, useState } from "react";
import { useAgent } from "agents/react";
import { useAgentChat } from "@cloudflare/ai-chat/react";
import { useAISDKRuntime } from "@assistant-ui/ai-sdk";
import {
  AssistantRuntimeProvider,
  ThreadPrimitive,
  MessagePrimitive,
  ComposerPrimitive,
  useAuiState,
} from "@assistant-ui/react";
import { MarkdownTextPrimitive } from "@assistant-ui/react-markdown";
import "./style.css";

type ChatSession = {
  model: string;
  conversation: string;
  draftKey: string;
  expiresAt: number;
};
type SessionGate = "loading" | "unauthenticated" | "owner-only" | "unavailable" | "ready";
type ClerkLike = {
  loaded?: boolean;
  addListener(listener: (payload: { session?: { id?: string } | null }) => void): () => void;
  signOut(options?: { redirectUrl?: string }): void | Promise<void>;
};
type ClerkWindow = typeof window & { Clerk?: ClerkLike };

const SESSION_ENDPOINT = "/api/chat/session";
const DRAFT_PREFIX = "gog:chat:draft:";
const draftStorageKey = (draftKey: string) => DRAFT_PREFIX + draftKey;
// Clerk is initialized by a script the server injects into the built HTML.
const clerk = (): ClerkLike | null => (window as ClerkWindow).Clerk ?? null;

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

function AssistantMessage() {
  return (
    <MessagePrimitive.Root className="message assistant">
      <span className="speaker">Gog</span>
      <MessagePrimitive.Parts components={{ Text: Markdown }} />
    </MessagePrimitive.Root>
  );
}

function Chat({
  session,
  onSignOut,
  onDisconnect,
}: {
  session: ChatSession;
  onSignOut: () => void;
  onDisconnect: () => void;
}) {
  const [connected, setConnected] = useState(false);
  const [hasDraft, setHasDraft] = useState(false);
  const [keyboardFocus, setKeyboardFocus] = useState(false);
  const [draftSaved, setDraftSaved] = useState(true);
  const [announcement, setAnnouncement] = useState("");
  const [turnNotice, setTurnNotice] = useState("");
  const draftKey = draftStorageKey(session.draftKey);
  const agent = useAgent({
    agent: "Chat",
    name: session.conversation,
    onOpen: () => setConnected(true),
    onClose: () => {
      setConnected(false);
      onDisconnect();
    },
  });
  const chat = useAgentChat({
    agent,
    onFinish: ({ message, isAbort, isError }) => {
      if (isAbort || isError) return;
      const text = message.parts
        .filter((p) => p.type === "text")
        .map((p) => p.text)
        .join(" ")
        .trim();
      setAnnouncement(
        "Gog replied: " +
          text.slice(0, 1200) +
          (text.length > 1200 ? " Full response is in the conversation." : ""),
      );
    },
    onTurnEnd: (event) => {
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
        if (text) window.sessionStorage.setItem(draftKey, text);
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
        <header>
          <a className="brand" href="/">
            gog-marketing<span> / private assistant</span>
          </a>
          <button className="quiet" onClick={onSignOut}>
            Sign out
          </button>
        </header>
        <div className="notice" role="note">
          Google tools are not connected yet. Open <a href="/#google-data">Google settings</a> or
          return to <a href="/">Home</a>.
        </div>
        <main aria-label="Chat">
          <ThreadPrimitive.Root className="thread">
            <ThreadPrimitive.Viewport
              className="viewport"
              role="region"
              aria-label="Conversation"
              tabIndex={0}
            >
              <ThreadPrimitive.Empty>
                <section className="empty">
                  <span className="eyebrow">SPACE TO THINK</span>
                  <h2>What are we working on?</h2>
                  <p>
                    Draft something, untangle a problem, or explore an idea.
                    <br />
                    Your sent messages are saved as you go.
                  </p>
                  <div className="suggestions">
                    {[
                      "Help me sharpen a campaign brief",
                      "Explain a tricky idea simply",
                      "Turn my notes into a clear plan",
                    ].map((prompt) => (
                      <ThreadPrimitive.Suggestion key={prompt} prompt={prompt} send>
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
                  placeholder="Ask anything, or bring an idea…"
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
                    disabled={!connected}
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
                <span>OpenRouter · {session.model}</span>
                <span>AI can make mistakes. Check important details.</span>
              </div>
            </div>
          </ThreadPrimitive.Root>
        </main>
        <footer>Your conversation is private. Google tools are not connected yet.</footer>
      </div>
    </AssistantRuntimeProvider>
  );
}

function App() {
  const [session, setSession] = useState<ChatSession | null>(null);
  const [gate, setGate] = useState<SessionGate>("loading");
  const [message, setMessage] = useState("");
  const applySession = useCallback((data: ChatSession) => {
    setSession(data);
    setGate("ready");
    setMessage("");
  }, []);
  const signOutView = useCallback((notice = "") => {
    setSession(null);
    setGate("unauthenticated");
    setMessage(notice);
  }, []);
  const ownerOnlyView = useCallback((notice = "") => {
    setSession(null);
    setGate("owner-only");
    setMessage(notice);
  }, []);
  const loadSession = useCallback(async () => {
    setGate("loading");
    setMessage("");
    try {
      const r = await fetch(SESSION_ENDPOINT, { signal: AbortSignal.timeout(10000) });
      if (r.ok) applySession((await r.json()) as ChatSession);
      else if (r.status === 401) signOutView();
      else if (r.status === 403) ownerOnlyView();
      else throw new Error("Session check failed");
    } catch {
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
    try {
      const r = await fetch(SESSION_ENDPOINT, { signal: AbortSignal.timeout(10000) });
      if (r.ok) {
        const data = (await r.json()) as ChatSession;
        setSession((current) => {
          if (!current) return null;
          // Same conversation: refresh fields in place, keeping the socket mounted.
          if (current.conversation === data.conversation && current.draftKey === data.draftKey)
            return { ...current, model: data.model, expiresAt: data.expiresAt };
          return data;
        });
        setMessage("");
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
  }, [signOutView, ownerOnlyView]);
  const signOut = useCallback(() => {
    // Close the socket before sign-out by unmounting the conversation; the draft stays.
    signOutView();
    const c = clerk();
    if (c?.signOut) void c.signOut({ redirectUrl: "/" });
  }, [signOutView]);
  useEffect(() => {
    void loadSession();
  }, [loadSession]);
  // Clerk session changes (sign-out elsewhere, session expiry) close the live socket
  // by dropping the mounted conversation until the user signs back in.
  useEffect(() => {
    let unlisten: (() => void) | undefined;
    let attempts = 0;
    const attach = window.setInterval(() => {
      const c = clerk();
      if (c?.loaded && !unlisten) {
        unlisten = c.addListener((payload) => {
          if (!payload.session)
            signOutView(
              "Your session expired or was signed out. Sign in again to continue your saved conversation and draft.",
            );
        });
      }
      if (c?.loaded || ++attempts > 20) window.clearInterval(attach);
    }, 500);
    return () => {
      window.clearInterval(attach);
      unlisten?.();
    };
  }, [signOutView]);
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
          <h1>Workspace unavailable</h1>
          <p role="alert">{message}</p>
          <button type="button" onClick={() => void loadSession()}>
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
          <span className="eyebrow">OWNER ONLY</span>
          <h1>Workspace unavailable</h1>
          <p role="alert">
            {message || "This workspace is available only to the workspace owner."}
          </p>
          <a className="signin-link" href="/">
            Home
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
          <p>
            {message ||
              "An owner-only workspace. Sign in to pick up where you left off. Your unsent draft stays in this tab."}
          </p>
          <a className="signin-link" href="/chat">
            Sign in to continue
          </a>
          <small>Access is restricted. Provider credentials stay on the server.</small>
        </section>
      </main>
    );
  return (
    <Chat key={session.conversation} session={session} onSignOut={signOut} onDisconnect={recheck} />
  );
}

createRoot(document.getElementById("root")!).render(<App />);
