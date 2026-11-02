import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { SETTINGS_SECTIONS, type SettingsSectionId } from "../../product-navigation.js";
import { Button, Icon, LinkButton, SaveBar } from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { LoadError, LoadingPanel } from "../states.js";
import {
  attentionBySection,
  enabledCount,
  pendingSummary,
  plural,
  toolSummary,
  type AttentionItem,
} from "../model.js";

/** Settings shell: a secondary nav whose items show what needs attention, plus the save bar. */
export function SettingsLayout({
  section,
  children,
}: {
  section: SettingsSectionId;
  children: ReactNode;
}) {
  const ws = useWorkspace();
  const counts = attentionBySection(ws.attention);
  const dirty = ws.assetChanges > 0 || ws.toolsDirty;
  const selectedCount = Array.from(ws.draft.values()).filter(Boolean).length;
  return (
    <div className="settings-layout">
      <nav className="settings-nav" aria-label="Settings sections">
        <p className="settings-nav-title" aria-hidden="true">
          Settings
        </p>
        <ul>
          {SETTINGS_SECTIONS.map((item) => {
            const count = item.id === "overview" ? ws.attention.length : (counts[item.id] ?? 0);
            return (
              <li key={item.id}>
                <Link
                  href={item.href}
                  aria-current={item.id === section ? "page" : undefined}
                  className="settings-nav-link"
                >
                  <span>{item.label}</span>
                  {count > 0 && item.id !== "overview" && (
                    <span className="settings-nav-flag">
                      <Icon name="alert" />
                      <span className="sr-only">: needs attention</span>
                    </span>
                  )}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>
      <div className="settings-content">
        {children}
        {dirty && (
          <SaveBar
            summary={`${selectedCount} ${plural(selectedCount, "asset", "assets")} selected`}
            detail={pendingSummary(ws.assetChanges, ws.toolsDirty)}
            onSave={ws.saveAll}
            onCancel={ws.discardAll}
            isSaving={ws.saving}
          />
        )}
      </div>
    </div>
  );
}

export function AttentionList({ items, limit }: { items: AttentionItem[]; limit?: number }) {
  const ws = useWorkspace();
  const shown = limit ? items.slice(0, limit) : items;
  return (
    <ul className="attention-list">
      {shown.map((item) => (
        <li key={item.id} className={"attention-item " + item.level}>
          <span className="attention-icon">
            <Icon name={item.level === "action" ? "alert" : "circle"} />
          </span>
          <div className="attention-copy">
            <strong>{item.title}</strong>
            <p>{item.detail}</p>
          </div>
          {item.action.href ? (
            <LinkButton
              variant={item.level === "action" ? "primary" : "secondary"}
              href={item.action.href}
            >
              {item.action.label}
            </LinkButton>
          ) : (
            <Button
              onPress={() => {
                const connection = ws.connections.find((c) => c.id === item.action.discover);
                if (connection) void ws.runAction("Asset check", connection, "discover");
              }}
            >
              {item.action.label}
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}

function formatWhen(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime())
    ? ""
    : date.toLocaleString(undefined, {
        day: "numeric",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
      });
}

export function SettingsOverview() {
  const ws = useWorkspace();
  const counts = attentionBySection(ws.attention);
  if (ws.loadState === "loading") return <LoadingPanel />;
  if (ws.loadState === "error") return <LoadError onRetry={ws.reload} />;

  const accounts = ws.connections.length;
  const assets = ws.connections.reduce((sum, item) => sum + enabledCount(item), 0);
  const found = ws.connections.reduce((sum, item) => sum + (item.resources ?? []).length, 0);
  const tools = ws.connections.reduce(
    (sum, item) => {
      const catalog = ws.toolCatalogs.get(item.id);
      if (!catalog) return sum;
      const summary = toolSummary(item, catalog);
      return { enabled: sum.enabled + summary.enabled, changes: sum.changes + summary.changes };
    },
    { enabled: 0, changes: 0 },
  );
  const loadedTools = ws.connections.every((item) => ws.toolCatalogs.has(item.id));
  const last = ws.activity?.[0];
  const unsaved = ws.assetChanges > 0 || ws.toolsDirty;

  const rows: Array<{
    id: string;
    label: string;
    value: string;
    href: string;
    action: string;
  }> = [
    {
      id: "accounts",
      label: "Google accounts",
      value: accounts
        ? `${accounts} connected · ${ws.connections
            .map((item) => item.email ?? "")
            .filter(Boolean)
            .join(", ")}`
        : "None connected",
      href: "/settings/accounts",
      action: accounts ? "Manage accounts" : "Connect account",
    },
    {
      id: "data",
      label: "Data access",
      value: accounts ? `${assets} of ${found} assets enabled` : "Connect an account first",
      href: "/settings/data",
      action: "Choose data",
    },
    {
      id: "tools",
      label: "AI tools",
      value: loadedTools
        ? `${tools.enabled} enabled · ${tools.changes} can change Google data`
        : "Loading…",
      href: "/settings/tools",
      action: "Choose tools",
    },
    {
      id: "activity",
      label: "Activity",
      value:
        ws.activityState === "error"
          ? "Couldn’t load activity"
          : last
            ? `Last: ${last.title} · ${formatWhen(last.at)}`
            : ws.activityState === "loading"
              ? "Loading…"
              : "No activity yet",
      href: "/settings/activity",
      action: "View activity",
    },
    {
      id: "connect-ai",
      label: "Connect AI apps",
      value: "Use your Google data from ChatGPT, Claude or Codex",
      href: "/settings/connect-ai",
      action: "Set up",
    },
  ];

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Settings</h1>
          <p>What AI can access, and anything that needs you.</p>
        </div>
      </header>

      <section
        className={"panel attention-panel" + (ws.attention.length ? "" : " is-clear")}
        aria-labelledby="attention-heading"
      >
        {ws.attention.length || unsaved ? (
          <>
            <div className="panel-head">
              <div>
                <h2 id="attention-heading">
                  {ws.attention.length
                    ? `${ws.attention.length} ${plural(ws.attention.length, "thing needs", "things need")} your attention`
                    : "Unsaved changes"}
                </h2>
                <p>Most urgent first. Everything else is working.</p>
              </div>
            </div>
            {unsaved && (
              <p className="attention-unsaved">
                <Icon name="alert" /> You have unsaved changes. Save or cancel them with the bar at
                the bottom of the page.
              </p>
            )}
            <AttentionList items={ws.attention} />
          </>
        ) : (
          <div className="attention-clear">
            <span className="attention-icon ok">
              <Icon name="check" />
            </span>
            <div>
              <h2 id="attention-heading">Nothing needs your attention</h2>
              <p>Your accounts are connected, and AI tools can use the data you enabled.</p>
            </div>
          </div>
        )}
      </section>

      <section className="panel" aria-labelledby="summary-heading">
        <div className="panel-head">
          <div>
            <h2 id="summary-heading">Your settings</h2>
            <p>A summary of each area. Open one to change it.</p>
          </div>
        </div>
        <dl className="settings-summary">
          {rows.map((row) => {
            const count = counts[row.id] ?? 0;
            return (
              <div key={row.id} className="settings-summary-row">
                <dt>{row.label}</dt>
                <dd className="settings-summary-value">
                  <span>{row.value}</span>
                  {count > 0 && (
                    <span className="status needs_attention">
                      {count} {plural(count, "item needs", "items need")} attention
                    </span>
                  )}
                </dd>
                <dd className="settings-summary-action">
                  <Link href={row.href} aria-label={`${row.action}: ${row.label}`}>
                    {row.action}
                    <Icon name="arrow" />
                  </Link>
                </dd>
              </div>
            );
          })}
        </dl>
      </section>
    </>
  );
}
