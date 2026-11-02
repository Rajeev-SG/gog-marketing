import { useEffect, useMemo, useState } from "react";
import {
  Button as AriaButton,
  Disclosure as AriaDisclosure,
  DisclosurePanel,
  Tab,
  TabList,
  TabPanel,
  Tabs,
} from "react-aria-components";
import {
  ActionMenu,
  Button,
  Checkbox,
  Disclosure,
  LinkButton,
  SaveBar,
  SearchField,
  SegmentedControl,
  StatusBadge,
  useConfirm,
} from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { navigate, useLocation } from "../router.js";
import { ServiceMark } from "../ServiceMark.js";
import { LoadError, LoadingPanel } from "../states.js";
import { openPicker } from "../picker.js";
import {
  accountName,
  category,
  discoveryDelayed,
  enableVisibleReadTools,
  enabledCount,
  filteredToolCatalog,
  matchesAsset,
  pendingSummary,
  plural,
  readableToolLabel,
  resourceKey,
  safeStatus,
  serviceGroups,
  serviceLabel,
  toolGroups,
  type Connection,
  type Tool,
  type ToolFilter,
} from "../model.js";

export function DataPage() {
  const ws = useWorkspace();
  const url = useLocation();
  const requestedAccount = url.searchParams.get("account");
  useEffect(() => {
    if (requestedAccount && ws.loadState === "ready") void ws.selectAccount(requestedAccount);
    // Only when the link target changes or the first load finishes.
  }, [requestedAccount, ws.loadState]);

  const selectedCount = Array.from(ws.draft.values()).filter(Boolean).length;
  const dirty = ws.assetChanges > 0 || ws.toolsDirty;

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Google data</h1>
          <p>Choose which assets and tools your AI agents can use, one Google account at a time.</p>
        </div>
        <div className="page-actions">
          <LinkButton href="/access#connect">Add Google account</LinkButton>
        </div>
      </header>
      {ws.loadState === "loading" && <LoadingPanel />}
      {ws.loadState === "error" && <LoadError onRetry={ws.reload} />}
      {ws.loadState === "ready" && ws.connections.length === 0 && (
        <section className="panel empty">
          <h2>No Google accounts connected yet</h2>
          <p>
            Connect an account, then choose the exact data AI tools may use. Nothing is enabled
            automatically.
          </p>
          <LinkButton variant="primary" href="/access#connect">
            Connect Google account
          </LinkButton>
        </section>
      )}
      {ws.loadState === "ready" && ws.connections.length > 0 && (
        <>
          <AccountList />
          {ws.selected && (
            <AssetPanel
              connection={ws.selected}
              initialService={url.searchParams.get("service")}
              initialQuery={url.searchParams.get("q") ?? ""}
            />
          )}
          {ws.selected && <ToolPanel connection={ws.selected} />}
          {dirty && (
            <SaveBar
              summary={`${selectedCount} ${plural(selectedCount, "asset", "assets")} selected`}
              detail={pendingSummary(ws.assetChanges, ws.toolsDirty)}
              onSave={ws.saveAll}
              onCancel={ws.discardAll}
              isSaving={ws.saving}
            />
          )}
        </>
      )}
    </>
  );
}

function AccountList() {
  const ws = useWorkspace();
  return (
    <section className="panel" aria-labelledby="accounts-heading">
      <div className="panel-head">
        <div>
          <h2 id="accounts-heading">Google accounts</h2>
          <p>Choose an account to manage its data and tools.</p>
        </div>
      </div>
      <ul className="account-list-cards">
        {ws.connections.map((connection) => {
          const [tone, text] = safeStatus(connection);
          const current = connection.id === ws.selected?.id;
          const who = accountName(connection);
          return (
            <li
              key={connection.id}
              className={"card account-card" + (current ? " is-selected" : "")}
              aria-current={current ? "true" : undefined}
            >
              <span className="avatar" aria-hidden="true">
                {who.slice(0, 1).toUpperCase()}
              </span>
              <div>
                <h3>{who}</h3>
                <p className="muted">
                  {(connection.services ?? []).length} services · {enabledCount(connection)} assets
                  enabled
                </p>
                {tone !== "ok" && (
                  <p className="account-status">
                    <StatusBadge tone={tone}>{text}</StatusBadge>
                  </p>
                )}
              </div>
              <div className="card-actions">
                {connection.status !== "active" && (
                  <Button
                    variant="attention"
                    onPress={() =>
                      navigate(`/access?update=${encodeURIComponent(connection.id)}#connect`)
                    }
                  >
                    Reconnect Google
                  </Button>
                )}
                {current ? (
                  <span className="selected-note">Managing now</span>
                ) : (
                  <Button variant="primary" onPress={() => void ws.selectAccount(connection.id)}>
                    Manage data access
                  </Button>
                )}
                <ActionMenu
                  label={`More actions for ${who}`}
                  heading={`Actions for ${who}`}
                  actions={[
                    {
                      id: "refresh",
                      label: "Refresh connection",
                      onAction: () => void ws.runAction("Refresh", connection, "refresh"),
                    },
                    {
                      id: "discover",
                      label: "Check assets",
                      onAction: () => void ws.runAction("Asset check", connection, "discover"),
                    },
                    {
                      id: "permissions",
                      label: "Update Google permissions",
                      onAction: () =>
                        navigate(`/access?update=${encodeURIComponent(connection.id)}#connect`),
                    },
                    {
                      id: "disconnect",
                      label: "Disconnect account",
                      danger: true,
                      onAction: () => void ws.runAction("Disconnect", connection, "disconnect"),
                    },
                  ]}
                />
              </div>
              {discoveryDelayed(connection) && (connection.resources ?? []).length > 0 && (
                <div className="partial-banner">
                  <div>
                    <strong>Some Google services couldn’t be checked</strong>
                    <p>Your existing selections are unchanged and remain available.</p>
                  </div>
                  <Button onPress={() => void ws.runAction("Asset check", connection, "discover")}>
                    Try again
                  </Button>
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function AssetPanel({
  connection,
  initialService,
  initialQuery,
}: {
  connection: Connection;
  initialService: string | null;
  initialQuery: string;
}) {
  const ws = useWorkspace();
  const groups = useMemo(() => serviceGroups(connection), [connection]);
  const [active, setActive] = useState<string>(initialService ?? "");
  const [query, setQuery] = useState(initialQuery);
  const [pickerStatus, setPickerStatus] = useState("");
  useEffect(() => setQuery(initialQuery), [initialQuery]);
  useEffect(() => {
    if (initialService) setActive(initialService);
  }, [initialService]);
  const activeService = groups.some((group) => group.service === active)
    ? active
    : (groups[0]?.service ?? "");
  const selected = Array.from(ws.draft.values()).filter(Boolean).length;
  const [tone, text] = safeStatus(connection);
  const empty = !(connection.resources ?? []).length;

  return (
    <section className="panel" id="assets" aria-labelledby="asset-heading">
      <div className="panel-head">
        <div>
          <p className="eyebrow">Google data</p>
          <h2 id="asset-heading">Manage asset access</h2>
          <p className="panel-account">Google account: {accountName(connection)}</p>
          <p>
            Choose the existing Google assets your AI agents and tools can access. This controls
            access, not reporting.
          </p>
        </div>
        <Button
          onPress={() =>
            void openPicker(connection, setPickerStatus, async () => {
              await ws.reload();
            })
          }
        >
          Choose Google files
        </Button>
      </div>
      {pickerStatus && (
        <p className="picker-status" role="status">
          {pickerStatus}
        </p>
      )}
      <div className="asset-frame">
        <div className="asset-toolbar">
          <SearchField
            label="Search assets"
            value={query}
            onChange={setQuery}
            placeholder="Search assets…"
            className="asset-search"
          />
          <div className="asset-summary">
            <strong>{selected} selected</strong>
            <StatusBadge tone={tone !== "ok" ? tone : empty ? "disconnected" : "ok"}>
              {tone !== "ok" ? text : empty ? "Connected · no assets yet" : "All services healthy"}
            </StatusBadge>
          </div>
        </div>
        {groups.length === 0 ? (
          <div className="empty">
            <h2>No assets found yet</h2>
            <p>This account is connected. Check again when Google has assets available.</p>
          </div>
        ) : (
          <Tabs
            orientation="vertical"
            selectedKey={activeService}
            onSelectionChange={(key) => setActive(String(key))}
            className="asset-picker"
          >
            <TabList aria-label="Google services" className="rail">
              {groups.map((group) => (
                <Tab key={group.service} id={group.service} className="rail-item">
                  <ServiceMark service={group.service} />
                  <span>{group.label}</span>
                  <span className="rail-count">{group.resources.length}</span>
                </Tab>
              ))}
            </TabList>
            {groups.map((group) => {
              const visible = group.resources.filter((resource) => matchesAsset(resource, query));
              const visibleOn = visible.filter((resource) => ws.draft.get(resourceKey(resource)));
              return (
                <TabPanel key={group.service} id={group.service} className="asset-content">
                  <div className="card-head">
                    <h3>{group.label}</h3>
                    <p className="service-meta">
                      {query.trim()
                        ? `Showing ${visible.length} of ${group.resources.length} assets matching “${query.trim()}”`
                        : `${group.resources.length} ${plural(group.resources.length, "asset", "assets")} available`}
                    </p>
                  </div>
                  {visible.length === 0 ? (
                    <div className="empty">
                      <h2>No matching assets</h2>
                      <p>No assets match this search. Existing selections are unchanged.</p>
                    </div>
                  ) : (
                    <>
                      {visible.length > 1 && (
                        <div className="select-all-row">
                          <Checkbox
                            isSelected={visibleOn.length === visible.length}
                            isIndeterminate={
                              visibleOn.length > 0 && visibleOn.length < visible.length
                            }
                            onChange={(on) =>
                              visible.forEach((resource) => ws.setAssetEnabled(resource, on))
                            }
                          >
                            {query.trim()
                              ? `All ${visible.length} matching assets`
                              : `All ${group.label} assets`}
                          </Checkbox>
                        </div>
                      )}
                      <ul className="rows" aria-label={`${group.label} assets`}>
                        {visible.map((resource) => (
                          <li key={resourceKey(resource)} className="row">
                            <Checkbox
                              isSelected={ws.draft.get(resourceKey(resource)) === true}
                              onChange={(on) => ws.setAssetEnabled(resource, on)}
                              description={
                                (resource.resourceType ?? "") +
                                (resource.parent ? " · " + resource.parent : "")
                              }
                            >
                              {resource.displayName || resource.resourceId}
                            </Checkbox>
                          </li>
                        ))}
                      </ul>
                    </>
                  )}
                </TabPanel>
              );
            })}
          </Tabs>
        )}
      </div>
    </section>
  );
}

const FILTERS = [
  { id: "all", label: "All" },
  { id: "reads", label: "Reads" },
  { id: "changes", label: "Changes" },
] as const;

function ToolPanel({ connection }: { connection: Connection }) {
  const ws = useWorkspace();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<ToolFilter>("all");
  useEffect(() => {
    setSearch("");
    setFilter("all");
  }, [connection.id]);
  const who = accountName(connection);
  const visible = filteredToolCatalog(ws.tools, search, filter);
  const enabledVisible = visible.filter((tool) => ws.toolChoices.get(tool.name)?.enabled).length;
  const ready = ws.toolState === "ready";
  const canEnableReads = visible.some(
    (tool) => !tool.write && !ws.toolChoices.get(tool.name)?.enabled,
  );

  const toggleTool = async (tool: Tool, on: boolean) => {
    if (on && tool.write) {
      const ok = await confirm({
        title: "Allow this tool to change Google data?",
        body: (
          <>
            <p>
              <strong>{readableToolLabel(tool)}</strong> will be able to change data in {who}. Each
              change is still shown to you for approval before it runs.
            </p>
            <p className="tool-machine-id">{tool.name}</p>
          </>
        ),
        confirmLabel: "Allow changes",
      });
      if (!ok) return;
    }
    ws.setToolChoice(
      tool.name,
      on ? { enabled: true } : { enabled: false, allowDestructive: false },
    );
  };
  const toggleDestructive = async (tool: Tool, on: boolean) => {
    if (on) {
      const ok = await confirm({
        title: "Allow deleting or replacing data?",
        body: (
          <p>
            <strong>{readableToolLabel(tool)}</strong> could delete or overwrite data in {who}. This
            is a separate permission for this account only, and each action still needs your
            approval.
          </p>
        ),
        confirmLabel: "Allow destructive actions",
        destructive: true,
      });
      if (!ok) return;
    }
    ws.setToolChoice(tool.name, { allowDestructive: on });
  };

  return (
    <section className="panel" id="tools" aria-labelledby="tool-heading">
      <div className="panel-head">
        <div>
          <h2 id="tool-heading">AI tool permissions</h2>
          <p className="panel-account">Google account: {who}</p>
          <p>
            Select exactly which tools this Google account may run. Connecting Google and choosing
            assets never enables writes.
          </p>
        </div>
      </div>
      <div className="tool-panel">
        <div className="tool-toolbar">
          <SearchField
            label="Search tools"
            value={search}
            onChange={setSearch}
            placeholder="Search tools…"
            isDisabled={!ready}
            className="tool-search"
          />
          <SegmentedControl
            label="Filter tools by access type"
            options={FILTERS}
            value={filter}
            onChange={setFilter}
            isDisabled={!ready}
          />
          <Button
            isDisabled={!ready || !canEnableReads}
            onPress={() => {
              const choices = new Map(
                [...ws.toolChoices].map(([name, choice]) => [name, { ...choice }]),
              );
              const changed = enableVisibleReadTools(choices, ws.tools, search, filter);
              if (changed)
                ws.replaceToolChoices(
                  choices,
                  `Enabled ${changed} read-only ${plural(changed, "tool", "tools")} in this list. Write and destructive choices are unchanged.`,
                );
            }}
          >
            Enable read-only tools in this list
          </Button>
        </div>
        <p className="tool-scope-note">
          “Enable read-only tools in this list” affects only read-only tools shown by the current
          search and filter. It never enables write tools or destructive operations, and it leaves
          every write and destructive choice unchanged.
        </p>
        {ready && (
          <p className="tool-summary" role="status">
            Showing {visible.length} of {ws.tools.length} tools · {enabledVisible} enabled
          </p>
        )}
        {ws.toolState === "loading" && (
          <section className="tool-state" aria-busy="true">
            <h3>Loading tool permissions…</h3>
            <p>Your existing permissions are unchanged.</p>
          </section>
        )}
        {ws.toolState === "error" && (
          <section className="tool-state" role="alert">
            <h3>We couldn’t load tool permissions</h3>
            <p>Your existing permissions for {who} have not changed.</p>
            <Button variant="primary" onPress={() => void ws.reloadTools()}>
              Try again
            </Button>
          </section>
        )}
        {ready && ws.tools.length === 0 && (
          <section className="tool-state">
            <h3>No tools are available</h3>
            <p>This Google account has no tools in the hosted catalogue.</p>
          </section>
        )}
        {ready && ws.tools.length > 0 && visible.length === 0 && (
          <section className="tool-state">
            <h3>No matching tools</h3>
            <p>
              {search
                ? `No tools match “${search}” and your current filter. Existing permissions are unchanged.`
                : "No tools match your current filter. Existing permissions are unchanged."}
            </p>
          </section>
        )}
        {ready && visible.length > 0 && (
          <div className="tool-list" aria-label={`Tools for ${who}`} role="group">
            {toolGroups(visible).map((group) => {
              const enabled = group.tools.filter(
                (tool) => ws.toolChoices.get(tool.name)?.enabled,
              ).length;
              const canEdit = group.tools.filter((tool) => tool.write).length;
              return (
                <Disclosure
                  key={group.service}
                  className="tool-group"
                  defaultExpanded={Boolean(search)}
                  title={`${serviceLabel(group.service)} (${category(group.service)})`}
                  meta={`${group.tools.length} ${plural(group.tools.length, "tool", "tools")} · ${enabled} enabled · ${canEdit} can edit`}
                >
                  <ul className="tool-group-rows">
                    {group.tools.map((tool) => {
                      const choice = ws.toolChoices.get(tool.name);
                      if (!choice) return null;
                      return (
                        <li key={tool.name} className="tool-choice">
                          <Checkbox
                            isSelected={choice.enabled}
                            onChange={(on) => void toggleTool(tool, on)}
                            description={
                              <>
                                <span className="tool-machine-id">{tool.name}</span>
                                <span className={"tool-kind " + (tool.write ? "changes" : "reads")}>
                                  {tool.write ? "Changes Google data" : "Read only"}
                                </span>
                              </>
                            }
                          >
                            {readableToolLabel(tool)}
                          </Checkbox>
                          {tool.write && (
                            <Checkbox
                              className="tool-destructive"
                              aria-label={`Also allow deleting or replacing data with ${readableToolLabel(tool)}`}
                              isSelected={choice.allowDestructive}
                              isDisabled={!choice.enabled}
                              onChange={(on) => void toggleDestructive(tool, on)}
                            >
                              Also allow deleting or replacing data
                            </Checkbox>
                          )}
                          <AriaDisclosure className="tool-details">
                            <AriaButton slot="trigger" className="tool-details-trigger">
                              Technical description
                            </AriaButton>
                            <DisclosurePanel>
                              <p>{tool.description}</p>
                            </DisclosurePanel>
                          </AriaDisclosure>
                        </li>
                      );
                    })}
                  </ul>
                </Disclosure>
              );
            })}
          </div>
        )}
        {ws.toolStatus && (
          <p className="tool-status" role="status">
            {ws.toolStatus}
          </p>
        )}
      </div>
    </section>
  );
}
