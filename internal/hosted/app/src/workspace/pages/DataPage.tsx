import { useEffect, useMemo, useState } from "react";
import {
  Link,
  Button as AriaButton,
  Disclosure as AriaDisclosure,
  DisclosurePanel,
  Tab,
  TabList,
  TabPanel,
  Tabs,
} from "react-aria-components";
import {
  Button,
  Checkbox,
  Disclosure,
  LinkButton,
  SearchField,
  SegmentedControl,
  StatusBadge,
  useConfirm,
} from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { useLocation } from "../router.js";
import { AccountSwitcher } from "../AccountSwitcher.js";
import { AttentionList } from "./SettingsPage.js";
import { ServiceMark } from "../ServiceMark.js";
import { LoadError, LoadingPanel } from "../states.js";
import { openPicker } from "../picker.js";
import {
  accountName,
  enableVisibleReadTools,
  filteredToolCatalog,
  hiddenToolCount,
  matchesAsset,
  plural,
  readableToolLabel,
  toolActionLabel,
  resourceKey,
  safeStatus,
  serviceGroups,
  serviceLabel,
  visibleToolGroups,
  type Connection,
  type Tool,
  type ToolFilter,
} from "../model.js";

function useAccountFromUrl() {
  const ws = useWorkspace();
  const url = useLocation();
  const requestedAccount = url.searchParams.get("account");
  useEffect(() => {
    if (requestedAccount && ws.loadState === "ready") void ws.selectAccount(requestedAccount);
    // Only when the link target changes or the first load finishes.
  }, [requestedAccount, ws.loadState]);
  return url;
}

function NoAccounts() {
  return (
    <section className="panel empty">
      <h2>No Google accounts connected yet</h2>
      <p>
        Connect an account, then choose the exact data AI tools may use. Nothing is enabled
        automatically.
      </p>
      <LinkButton variant="primary" href="/settings/accounts#connect">
        Connect Google account
      </LinkButton>
    </section>
  );
}

/** Settings › Data access: which assets AI tools may use, per account. */
export function DataAccessPage() {
  const ws = useWorkspace();
  const url = useAccountFromUrl();
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Data access</h1>
          <p>Choose the Google assets AI tools may use. New assets start disabled.</p>
        </div>
        <AccountSwitcher />
      </header>
      {ws.loadState === "loading" && <LoadingPanel />}
      {ws.loadState === "error" && <LoadError onRetry={ws.reload} />}
      {ws.loadState === "ready" && ws.connections.length === 0 && <NoAccounts />}
      {ws.loadState === "ready" && ws.selected && (
        <>
          <AccountAlert connection={ws.selected} />
          <AssetPanel
            connection={ws.selected}
            initialService={url.searchParams.get("service")}
            initialQuery={url.searchParams.get("q") ?? ""}
          />
        </>
      )}
    </>
  );
}

/** Settings › AI tools: which tools may run for each account. */
export function ToolsPage() {
  const ws = useWorkspace();
  useAccountFromUrl();
  return (
    <>
      <header className="page-header">
        <div>
          <h1>AI tools</h1>
          <p>Choose which tools AI may run. Tools that change Google data always ask you first.</p>
        </div>
        <AccountSwitcher />
      </header>
      {ws.loadState === "loading" && <LoadingPanel />}
      {ws.loadState === "error" && <LoadError onRetry={ws.reload} />}
      {ws.loadState === "ready" && ws.connections.length === 0 && <NoAccounts />}
      {ws.loadState === "ready" && ws.selected && (
        <>
          <AccountAlert connection={ws.selected} />
          <ToolPanel connection={ws.selected} />
        </>
      )}
    </>
  );
}

/** The selected account's own attention items, shown where they can be fixed. */
function AccountAlert({ connection }: { connection: Connection }) {
  const ws = useWorkspace();
  const items = ws.attention.filter((item) => item.id.endsWith(":" + connection.id));
  if (!items.length) return null;
  return (
    <section className="panel attention-panel compact" aria-label="Needs attention">
      <AttentionList items={items} />
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

  return (
    <section className="panel" id="assets" aria-labelledby="asset-heading">
      <div className="panel-head">
        <div>
          <h2 id="asset-heading">Assets in {accountName(connection)}</h2>
          <p>
            Tick an asset to let AI tools use it. Drive files appear here only after you choose
            them.
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
            <strong>
              {selected} of {(connection.resources ?? []).length} enabled
            </strong>
            {tone !== "ok" && <StatusBadge tone={tone}>{text}</StatusBadge>}
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
                  <span className="rail-count">
                    {group.resources.filter((r) => ws.draft.get(resourceKey(r))).length}/
                    {group.resources.length}
                    <span className="sr-only"> enabled</span>
                  </span>
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
  const [showUnshared, setShowUnshared] = useState(false);
  useEffect(() => {
    setSearch("");
    setFilter("all");
    setShowUnshared(false);
  }, [connection.id]);
  const hidden = hiddenToolCount(connection, ws.tools);
  const who = accountName(connection);
  const groups = visibleToolGroups(
    connection,
    filteredToolCatalog(ws.tools, search, filter),
    ws.toolChoices,
    showUnshared,
  );
  const visible = groups.flatMap((group) => group.tools);
  const enabledVisible = visible.filter((tool) => ws.toolChoices.get(tool.name)?.enabled).length;
  const ready = ws.toolState === "ready";
  const canEnableReads = groups.some(
    (group) =>
      group.shared &&
      group.tools.some((tool) => !tool.write && !ws.toolChoices.get(tool.name)?.enabled),
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
          <h2 id="tool-heading">Tools for {who}</h2>
          <p>
            Tools work only on the data you enabled. Start with read-only tools; turn on tools that
            change data only when you need them.
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
              const changed = enableVisibleReadTools(
                choices,
                groups.filter((group) => group.shared).flatMap((group) => group.tools),
                "",
                "all",
              );
              if (changed)
                ws.replaceToolChoices(
                  choices,
                  `Enabled ${changed} read-only ${plural(changed, "tool", "tools")} in this list. Write and destructive choices are unchanged.`,
                );
            }}
          >
            Turn on all read-only tools shown
          </Button>
        </div>
        <p className="tool-scope-note">
          This only turns on read-only tools in the current list. It never turns on tools that
          change or delete data.
        </p>
        {ready && (
          <p className="tool-summary" role="status">
            Showing {visible.length} {plural(visible.length, "tool", "tools")} · {enabledVisible}{" "}
            enabled
          </p>
        )}
        {ready && hidden.tools > 0 && (
          <div className="tool-hidden-note">
            <p>
              {showUnshared ? "Also showing " : ""}
              {hidden.tools} {plural(hidden.tools, "tool", "tools")} for {hidden.services}{" "}
              {plural(hidden.services, "service", "services")} {who} doesn’t share
              {showUnshared
                ? ". They can’t run until the account shares that service."
                : hidden.tools === 1
                  ? " is hidden."
                  : " are hidden."}{" "}
              <Link href={`/settings/accounts?update=${encodeURIComponent(connection.id)}#connect`}>
                Change shared services
              </Link>
            </p>
            <Button size="small" variant="quiet" onPress={() => setShowUnshared((on) => !on)}>
              {showUnshared ? "Hide them" : "Show them"}
            </Button>
          </div>
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
            {groups.map((group) => {
              const enabled = group.tools.filter(
                (tool) => ws.toolChoices.get(tool.name)?.enabled,
              ).length;
              const canEdit = group.tools.filter((tool) => tool.write).length;
              return (
                <Disclosure
                  key={group.service}
                  className="tool-group"
                  defaultExpanded={Boolean(search)}
                  title={
                    <>
                      <ServiceMark service={group.service} />
                      {serviceLabel(group.service)}
                      {!group.shared && (
                        <span className="status needs_attention">Not shared · can’t run</span>
                      )}
                    </>
                  }
                  meta={`${enabled} of ${group.tools.length} on${canEdit ? ` · ${canEdit} can change data` : ""}`}
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
                            {toolActionLabel(tool)}
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
