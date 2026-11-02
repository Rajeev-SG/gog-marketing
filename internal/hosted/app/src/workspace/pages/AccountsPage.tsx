import { useState } from "react";
import { CONSENT_GROUPS, consentServiceIds } from "../../consent.js";
import { HOSTED_SERVICES } from "../../scopes.js";
import {
  ActionMenu,
  Button,
  Checkbox,
  Disclosure,
  Icon,
  LinkButton,
  Notice,
  StatusBadge,
} from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { navigate, useLocation } from "../router.js";
import { ServiceMark } from "../ServiceMark.js";
import { LoadError, LoadingPanel } from "../states.js";
import {
  accountName,
  discoveryDelayed,
  enabledCount,
  plural,
  requestConnection,
  safeStatus,
  serviceKey,
  toolSummary,
  type Connection,
} from "../model.js";

export function AccountsPage() {
  const ws = useWorkspace();
  const url = useLocation();
  const target = ws.connections.find((item) => item.id === url.searchParams.get("update"));

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Google accounts</h1>
          <p>Each connected account, what it shares, and how to add or change that.</p>
        </div>
        <div className="page-actions">
          <LinkButton variant="primary" href="/settings/accounts#connect">
            <Icon name="plus" /> Add Google account
          </LinkButton>
        </div>
      </header>

      {ws.loadState === "loading" && <LoadingPanel />}
      {ws.loadState === "error" && <LoadError onRetry={ws.reload} />}
      {ws.loadState === "ready" && ws.connections.length > 0 && (
        <section className="panel" aria-labelledby="accounts-heading">
          <div className="panel-head">
            <div>
              <h2 id="accounts-heading">Connected accounts</h2>
              <p>AI tools can use an account only for the data and tools you enable.</p>
            </div>
          </div>
          <ul className="account-list-cards">
            {ws.connections.map((connection) => (
              <AccountCard key={connection.id} connection={connection} />
            ))}
          </ul>
        </section>
      )}

      <section className="access-strip" aria-labelledby="how-heading">
        <Icon name="shield" />
        <div>
          <h2 id="how-heading">How access works</h2>
          <ol className="access-steps-inline">
            <li>
              <strong>Share services.</strong> When you connect, you choose which Google services
              the account shares. Reading and changing data are separate choices.
            </li>
            <li>
              <strong>Enable data.</strong> AI tools see only the assets you enable in Data access.
              New assets start disabled.
            </li>
            <li>
              <strong>Enable tools.</strong> Each AI tool is enabled per account. Every Google
              change is shown to you before it runs.
            </li>
          </ol>
        </div>
      </section>

      <ConnectForm
        key={target?.id ?? "new"}
        targetId={target?.id ?? null}
        targetName={target ? accountName(target) : ""}
      />
    </>
  );
}

function AccountCard({ connection }: { connection: Connection }) {
  const ws = useWorkspace();
  const [tone, text] = safeStatus(connection);
  const who = accountName(connection);
  const catalog = ws.toolCatalogs.get(connection.id);
  const tools = catalog ? toolSummary(connection, catalog) : null;
  const services = connection.services ?? [];
  const reconnect = connection.status !== "active";
  const update = `/settings/accounts?update=${encodeURIComponent(connection.id)}#connect`;
  const account = encodeURIComponent(connection.id);
  return (
    <li className={"card account-card" + (tone === "needs_attention" ? " needs-attention" : "")}>
      <span className="avatar" aria-hidden="true">
        {who.slice(0, 1).toUpperCase()}
      </span>
      <div>
        <h3>{who}</h3>
        <p className="muted">
          {services.length} {plural(services.length, "service", "services")} shared ·{" "}
          {enabledCount(connection)} of {(connection.resources ?? []).length} assets enabled
          {tools ? ` · ${tools.enabled} AI ${plural(tools.enabled, "tool", "tools")} enabled` : ""}
        </p>
        {tone !== "ok" && (
          <p className="account-status">
            <StatusBadge tone={tone}>{text}</StatusBadge>
          </p>
        )}
      </div>
      <div className="card-actions">
        {reconnect && (
          <LinkButton variant="attention" href={update}>
            Reconnect Google
          </LinkButton>
        )}
        <LinkButton href={`/settings/data?account=${account}`}>Data</LinkButton>
        <LinkButton href={`/settings/tools?account=${account}`}>AI tools</LinkButton>
        <ActionMenu
          label={`More actions for ${who}`}
          heading={`Actions for ${who}`}
          actions={[
            {
              id: "permissions",
              label: "Change shared services",
              onAction: () => navigate(update),
            },
            {
              id: "discover",
              label: "Check for new assets",
              onAction: () => void ws.runAction("Asset check", connection, "discover"),
            },
            {
              id: "refresh",
              label: "Refresh connection",
              onAction: () => void ws.runAction("Refresh", connection, "refresh"),
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
      {services.length > 0 && (
        <Disclosure
          className="granted-disclosure"
          title="Shared services"
          meta={`${services.length} ${plural(services.length, "service", "services")}`}
        >
          <ul className="granted-services" aria-label={`Services shared by ${who}`}>
            {services.map((name) => (
              <li key={name}>
                <ServiceMark service={serviceKey(name)} />
                {name}
              </li>
            ))}
          </ul>
        </Disclosure>
      )}
      {discoveryDelayed(connection) && (connection.resources ?? []).length > 0 && (
        <div className="partial-banner">
          <div>
            <strong>Some Google services couldn’t be checked</strong>
            <p>Your existing selections are unchanged and remain available.</p>
          </div>
          <Button onPress={() => void ws.runAction("Asset check", connection, "discover")}>
            Check again
          </Button>
        </div>
      )}
    </li>
  );
}

function ConnectForm({ targetId, targetName }: { targetId: string | null; targetName: string }) {
  const [read, setRead] = useState<Set<string>>(new Set());
  const [write, setWrite] = useState<Set<string>>(new Set());
  const [pending, setPending] = useState(false);
  const [status, setStatus] = useState<{ tone: "info" | "error"; text: string } | null>(null);
  const all = consentServiceIds();

  const setReadFor = (service: string, on: boolean) => {
    setRead((current) => {
      const next = new Set(current);
      if (on) next.add(service);
      else next.delete(service);
      return next;
    });
    // Write consent depends on read consent: clearing read clears write.
    if (!on)
      setWrite((current) => {
        const next = new Set(current);
        next.delete(service);
        return next;
      });
  };
  const setWriteFor = (service: string, on: boolean) =>
    setWrite((current) => {
      const next = new Set(current);
      if (on) next.add(service);
      else next.delete(service);
      return next;
    });

  const submit = async () => {
    if (pending) return;
    if (!read.size) {
      setStatus({ tone: "error", text: "Choose at least one Google service to share." });
      return;
    }
    setPending(true);
    setStatus({ tone: "info", text: "Starting Google sign-in…" });
    try {
      const authorizationUrl = await requestConnection({
        services: [...read],
        writeServices: [...write],
        ...(targetId ? { connectionId: targetId } : {}),
      });
      window.location.assign(authorizationUrl);
    } catch (error) {
      setStatus({
        tone: "error",
        text: error instanceof Error ? error.message : "Could not start Google connection.",
      });
    } finally {
      setPending(false);
    }
  };

  return (
    <section className="panel connect-panel" id="connect" aria-labelledby="connect-heading">
      <div className="connect-layout">
        <div>
          <p className="eyebrow">{targetId ? "Change shared services" : "Add an account"}</p>
          <h2 id="connect-heading">
            {targetId ? `Change shared services for ${targetName}` : "Connect a Google account"}
          </h2>
          {targetId ? (
            <Notice tone="info">
              Changing what {targetName} shares. Services it already shares stay shared; select any
              to add, then continue to Google.{" "}
              <LinkButton href="/settings/accounts#connect">
                Connect a different account instead
              </LinkButton>
            </Notice>
          ) : (
            <p className="connect-help">
              Select only what this account should use. Changes need a separate opt-in per service;
              agents start with no assets enabled.
            </p>
          )}
          <div className="connect-consent-toolbar">
            <Button onPress={() => setRead(new Set(all))}>Select all services</Button>
            <Button
              onPress={() => {
                setRead(new Set());
                setWrite(new Set());
              }}
            >
              Clear selection
            </Button>
          </div>
          <p className="connect-consent-summary">
            Tick a service to let gog-marketing read it; tick Writes to also allow changes. Sharing
            a service doesn’t enable any assets or AI tools on its own.
          </p>
          <div
            className="service-consent"
            aria-label="Google service read and write consent"
            role="group"
          >
            {CONSENT_GROUPS.map((group) => (
              <fieldset
                key={group.legend}
                className="service-consent-group"
                data-consent-group={group.legend}
              >
                <legend>{group.legend}</legend>
                <div className="service-consent-list">
                  {group.services.map((service) => {
                    const info = HOSTED_SERVICES[service];
                    const reading = read.has(service);
                    return (
                      <div className="service-choice" data-service={service} key={service}>
                        <Checkbox
                          isSelected={reading}
                          onChange={(on) => setReadFor(service, on)}
                          className="service-choice-read"
                        >
                          {info.label}
                        </Checkbox>
                        {info.writeScopes.length ? (
                          <Checkbox
                            isSelected={write.has(service)}
                            isDisabled={!reading}
                            onChange={(on) => setWriteFor(service, on)}
                            aria-label={`Writes for ${info.label}`}
                            className="service-choice-write"
                          >
                            Writes
                          </Checkbox>
                        ) : (
                          <span className="service-choice-note">Read only</span>
                        )}
                      </div>
                    );
                  })}
                </div>
              </fieldset>
            ))}
          </div>
          <div className="connect-actions">
            <Button variant="primary" onPress={submit} isPending={pending}>
              <Icon name="plus" />
              {targetId ? "Continue to Google" : "Connect Google account"}
            </Button>
          </div>
          {status && (
            <p
              className={"connect-status " + status.tone}
              role={status.tone === "error" ? "alert" : "status"}
            >
              {status.text}
            </p>
          )}
        </div>
        <aside className="safety-note">
          <strong>Least privilege by default</strong>
          <p>
            Sharing a service doesn't make any tool available. You still choose exact assets and
            tools afterwards.
          </p>
        </aside>
      </div>
    </section>
  );
}
