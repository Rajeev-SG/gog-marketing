import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useConfirm, type NoticeTone } from "../ui/index.js";
import {
  accountName,
  changedGrants,
  resourceKey,
  toolGrantPayload,
  type Connection,
  type Tool,
  type ToolChoice,
} from "./model.js";

export type LoadState = "loading" | "error" | "ready";
export type ToolState = "idle" | "loading" | "error" | "ready";
export interface PageNotice {
  tone: NoticeTone;
  text: string;
}

interface WorkspaceValue {
  connections: Connection[];
  loadState: LoadState;
  reload: () => Promise<boolean>;
  selected: Connection | undefined;
  selectAccount: (id: string) => Promise<boolean>;
  notice: PageNotice | null;
  setNotice: (notice: PageNotice | null) => void;
  // Assets
  draft: ReadonlyMap<string, boolean>;
  setAssetEnabled: (resource: { service: string; resourceId: string }, enabled: boolean) => void;
  assetChanges: number;
  // Tools
  tools: Tool[];
  toolChoices: ReadonlyMap<string, ToolChoice>;
  toolState: ToolState;
  toolsDirty: boolean;
  toolStatus: string;
  setToolChoice: (name: string, change: Partial<ToolChoice>) => void;
  replaceToolChoices: (choices: Map<string, ToolChoice>, status: string) => void;
  reloadTools: () => Promise<void>;
  // One save model
  saveAll: () => Promise<void>;
  discardAll: () => void;
  saving: boolean;
  // Account actions
  runAction: (
    label: string,
    connection: Connection,
    action: "refresh" | "discover" | "disconnect",
  ) => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceValue | null>(null);
export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("useWorkspace must be used inside WorkspaceProvider");
  return value;
}

const postJson = (url: string, body: unknown) =>
  fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });

async function errorMessage(response: Response, fallback: string): Promise<string> {
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  return body.error || fallback;
}

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const confirm = useConfirm();
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [notice, setNotice] = useState<PageNotice | null>(null);
  const [drafts, setDrafts] = useState<Map<string, Map<string, boolean>>>(new Map());
  const [tools, setTools] = useState<Tool[]>([]);
  const [toolChoices, setToolChoices] = useState<Map<string, ToolChoice>>(new Map());
  const [toolState, setToolState] = useState<ToolState>("idle");
  const [toolsDirty, setToolsDirty] = useState(false);
  const [toolStatus, setToolStatus] = useState("");
  const [saving, setSaving] = useState(false);
  const toolRequest = useRef(0);

  const selected = connections.find((item) => item.id === selectedId);

  const reload = useCallback(async () => {
    setLoadState((state) => (state === "ready" ? state : "loading"));
    try {
      const response = await fetch("/api/google/resources");
      if (!response.ok) throw new Error();
      const data = (await response.json()) as { connections?: Connection[] };
      const next = (data.connections ?? []).map((item) => ({
        ...item,
        discovery: item.discovery ?? {},
      }));
      setConnections(next);
      setSelectedId((current) =>
        current && next.some((item) => item.id === current) ? current : (next[0]?.id ?? null),
      );
      setLoadState("ready");
      return true;
    } catch {
      setLoadState("error");
      return false;
    }
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);

  const loadTools = useCallback(async (connectionId: string) => {
    const request = ++toolRequest.current;
    setToolState("loading");
    setTools([]);
    setToolChoices(new Map());
    setToolsDirty(false);
    try {
      const response = await fetch(
        "/api/google/tools?connectionId=" + encodeURIComponent(connectionId),
      );
      if (!response.ok) throw new Error("Could not load tool permissions.");
      const body = (await response.json()) as { tools?: Tool[] };
      if (request !== toolRequest.current) return;
      if (!Array.isArray(body.tools)) throw new Error("The tool permission response was invalid.");
      setTools(body.tools);
      setToolChoices(
        new Map(
          body.tools.map((tool) => [
            tool.name,
            { toolName: tool.name, enabled: tool.enabled, allowDestructive: tool.allowDestructive },
          ]),
        ),
      );
      setToolState("ready");
    } catch (error) {
      if (request !== toolRequest.current) return;
      setToolState("error");
      setToolStatus(
        error instanceof Error
          ? error.message
          : "Could not load tools. Existing grants are unchanged.",
      );
    }
  }, []);
  useEffect(() => {
    setToolStatus("");
    if (selectedId) void loadTools(selectedId);
    else setToolState("idle");
  }, [selectedId, loadTools]);

  const selectAccount = useCallback(
    async (id: string) => {
      if (id === selectedId) return true;
      if (toolsDirty) {
        const discard = await confirm({
          title: "Discard unsaved changes?",
          body: (
            <p>Your unsaved tool permission changes for {accountName(selected)} will be lost.</p>
          ),
          confirmLabel: "Discard changes",
          cancelLabel: "Keep editing",
          destructive: true,
        });
        if (!discard) return false;
      }
      setSelectedId(id);
      return true;
    },
    [confirm, selected, selectedId, toolsDirty],
  );

  const draft = useMemo(() => {
    const stored = selected ? drafts.get(selected.id) : undefined;
    const merged = new Map<string, boolean>();
    for (const resource of selected?.resources ?? [])
      merged.set(resourceKey(resource), stored?.get(resourceKey(resource)) ?? resource.enabled);
    return merged;
  }, [drafts, selected]);
  const assetChanges = changedGrants(selected, draft).length;

  const setAssetEnabled = useCallback(
    (resource: { service: string; resourceId: string }, enabled: boolean) => {
      if (!selected) return;
      setDrafts((current) => {
        const next = new Map(current);
        const forAccount = new Map(next.get(selected.id) ?? []);
        forAccount.set(resourceKey(resource), enabled);
        next.set(selected.id, forAccount);
        return next;
      });
    },
    [selected],
  );

  const setToolChoice = useCallback((name: string, change: Partial<ToolChoice>) => {
    setToolChoices((current) => {
      const next = new Map(current);
      const choice = next.get(name);
      if (choice) next.set(name, { ...choice, ...change });
      return next;
    });
    setToolsDirty(true);
    setToolStatus("Unsaved tool permission changes.");
  }, []);
  const replaceToolChoices = useCallback((choices: Map<string, ToolChoice>, status: string) => {
    setToolChoices(choices);
    setToolsDirty(true);
    setToolStatus(status);
  }, []);

  const discardAll = useCallback(() => {
    if (!selected) return;
    setDrafts((current) => {
      const next = new Map(current);
      next.delete(selected.id);
      return next;
    });
    if (toolsDirty) void loadTools(selected.id);
    setNotice({ tone: "info", text: "Unsaved changes discarded." });
  }, [loadTools, selected, toolsDirty]);

  // One save model (DESIGN.md): tools save first; a failed tool save stops before
  // the asset save, so no draft is lost.
  const saveAll = useCallback(async () => {
    if (!selected) return;
    const connectionId = selected.id;
    const grants = changedGrants(selected, draft);
    if (!grants.length && !toolsDirty) {
      setNotice({ tone: "info", text: "No changes to save." });
      return;
    }
    setSaving(true);
    setNotice(null);
    try {
      if (toolsDirty) {
        setToolStatus("Saving…");
        const response = await postJson("/api/google/tools", {
          connectionId,
          grants: toolGrantPayload(toolChoices),
        });
        if (!response.ok) {
          const text = await errorMessage(response, "Could not save tool permissions.");
          setToolStatus(text);
          setNotice({ tone: "error", text });
          return;
        }
        setToolsDirty(false);
        await loadTools(connectionId);
        setToolStatus("Tool permissions saved for this account.");
      }
      if (grants.length) {
        const response = await postJson("/api/google/grants", { connectionId, grants });
        if (!response.ok) {
          setNotice({
            tone: "error",
            text: await errorMessage(response, "Could not save asset access."),
          });
          return;
        }
        setDrafts((current) => {
          const next = new Map(current);
          next.delete(connectionId);
          return next;
        });
        await reload();
      }
      setNotice({ tone: "success", text: "Access saved." });
    } catch {
      setNotice({
        tone: "error",
        text: "Could not save. Check your connection and try again; nothing was lost.",
      });
    } finally {
      setSaving(false);
    }
  }, [draft, loadTools, reload, selected, toolChoices, toolsDirty]);

  const runAction = useCallback(
    async (
      label: string,
      connection: Connection,
      action: "refresh" | "discover" | "disconnect",
    ) => {
      if (action === "disconnect") {
        const ok = await confirm({
          title: `Disconnect ${accountName(connection)}?`,
          body: (
            <p>
              AI agents and tools lose access to this account's data straight away. Your saved
              choices are removed; you can connect the account again later.
            </p>
          ),
          confirmLabel: "Disconnect account",
          destructive: true,
        });
        if (!ok) return;
      }
      setNotice(null);
      try {
        const response = await postJson(
          `/api/google/connections/${encodeURIComponent(connection.id)}/${action}`,
          {},
        );
        if (!response.ok) throw new Error(await errorMessage(response, `${label} failed.`));
        if (await reload()) setNotice({ tone: "success", text: `${label} complete.` });
      } catch (error) {
        setNotice({
          tone: "error",
          text: error instanceof Error ? error.message : `${label} failed.`,
        });
      }
    },
    [confirm, reload],
  );

  const value: WorkspaceValue = {
    connections,
    loadState,
    reload,
    selected,
    selectAccount,
    notice,
    setNotice,
    draft,
    setAssetEnabled,
    assetChanges,
    tools,
    toolChoices,
    toolState,
    toolsDirty,
    toolStatus,
    setToolChoice,
    replaceToolChoices,
    reloadTools: () => (selected ? loadTools(selected.id) : Promise.resolve()),
    saveAll,
    discardAll,
    saving,
    runAction,
  };
  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}
