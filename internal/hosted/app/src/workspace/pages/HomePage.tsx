import { useEffect, useState } from "react";
import { Link } from "react-aria-components";
import { Button, Icon, LinkButton } from "../../ui/index.js";
import { AttentionList } from "./SettingsPage.js";
import { useWorkspace } from "../state.js";
import { navigate } from "../router.js";
import { ServiceMark } from "../ServiceMark.js";
import { LoadingPanel, LoadError } from "../states.js";
import {
  connectOutcome,
  enabledCount,
  plural,
  serviceOverview,
  setupStep,
  workspaceSummary,
} from "../model.js";

interface Completion {
  ready: boolean;
  connectionId: string | null;
  accounts: number;
  services: number;
  assets: number;
}

export function HomePage() {
  const ws = useWorkspace();
  const [completion, setCompletion] = useState<Completion | null>(null);
  const [outcome] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    return { code: params.get("google"), connection: params.get("connection") };
  });

  // Google OAuth returns to /?google=<outcome>&connection=<id> (#50 completion state).
  useEffect(() => {
    if (!outcome.code || ws.loadState !== "ready") return;
    const [text, tone] = connectOutcome(outcome.code);
    if (outcome.code === "connected") {
      const connection =
        ws.connections.find((item) => item.id === outcome.connection) ??
        ws.connections[ws.connections.length - 1];
      setCompletion({
        ready: Boolean(connection && enabledCount(connection) > 0),
        connectionId: connection?.id ?? null,
        accounts: ws.connections.length,
        services: connection?.services?.length ?? 0,
        assets: connection ? enabledCount(connection) : 0,
      });
    }
    ws.setNotice({ tone: tone === "error" ? "error" : tone === "info" ? "success" : "info", text });
    window.history.replaceState(null, "", "/");
    // Runs once, when the first load finishes after returning from Google.
  }, [ws.loadState]);

  const summary = workspaceSummary(ws.connections);
  // While setup is incomplete the guide shows the next step, so the attention
  // list keeps only what the guide doesn't already say.
  const setupIds = /^(connect$|data:|tools:)/;
  const rows = serviceOverview(ws.connections);
  const step = setupStep(
    ws.selected,
    ws.toolState === "ready" && [...ws.toolChoices.values()].some((choice) => choice.enabled),
  );
  const attention = step.complete
    ? ws.attention
    : ws.attention.filter((item) => !setupIds.test(item.id));

  return (
    <>
      {completion && (
        <section className="panel completion" aria-labelledby="completion-heading">
          <ol className="completion-progress" aria-label="Connection progress">
            <li className="progress-step done">Connect Google</li>
            <li className={"progress-step" + (completion.ready ? " done" : "")}>Choose data</li>
            <li className={"progress-step" + (completion.ready ? " done" : "")}>Ready</li>
          </ol>
          <div className="completion-body">
            <h2 id="completion-heading">
              {completion.ready
                ? "Your Google data is ready"
                : "Google connected — choose your data"}
            </h2>
            <p>
              {completion.ready
                ? "Only the assets you selected are available to connected AI tools."
                : "No assets are available to connected AI tools until you make a selection."}
            </p>
            <dl className="completion-metrics">
              <div className="completion-metric">
                <dt>accounts connected</dt>
                <dd>{completion.accounts}</dd>
              </div>
              <div className="completion-metric">
                <dt>services enabled</dt>
                <dd>{completion.services}</dd>
              </div>
              <div className="completion-metric">
                <dt>assets enabled</dt>
                <dd>{completion.assets}</dd>
              </div>
            </dl>
            <Button
              variant="primary"
              onPress={async () => {
                if (!completion.ready && completion.connectionId)
                  await ws.selectAccount(completion.connectionId);
                setCompletion(null);
                if (!completion.ready) navigate("/settings/data");
              }}
            >
              {completion.ready ? "Done" : "Choose data"}
            </Button>
          </div>
        </section>
      )}

      <header className="page-header">
        <div>
          <h1>Your Google marketing data</h1>
          <p>Secure access and permissions for AI agents and tools.</p>
        </div>
        <div className="page-actions">
          <LinkButton variant="primary" href="/chat">
            Open chat
          </LinkButton>
        </div>
      </header>

      {ws.loadState === "loading" && <LoadingPanel />}
      {ws.loadState === "error" && <LoadError onRetry={ws.reload} />}
      {ws.loadState === "ready" && (
        <>
          {attention.length > 0 && (
            <section className="panel attention-panel" aria-labelledby="home-attention-heading">
              <div className="panel-head">
                <div>
                  <h2 id="home-attention-heading">
                    {attention.length} {plural(attention.length, "thing needs", "things need")} your
                    attention
                  </h2>
                </div>
                {attention.length > 3 && (
                  <Link className="panel-link" href="/settings">
                    See all in Settings →
                  </Link>
                )}
              </div>
              <AttentionList items={attention} limit={3} />
            </section>
          )}

          <section className="metric-strip" aria-label="Workspace summary">
            <Metric icon="users" value={String(summary.accounts)} label={summary.accountsLabel} />
            <Metric
              icon="shield"
              value={String(summary.enabledAssets)}
              label={`of ${summary.totalAssets} assets enabled`}
            />
            <Metric icon="plug" value={String(summary.services)} label={summary.servicesLabel} />
            {ws.attention.length ? (
              <Metric
                icon="alert"
                value={`${ws.attention.length} to review`}
                label="See Settings for details"
              />
            ) : (
              <Metric icon="check" value={summary.health} label={summary.healthDetail} />
            )}
          </section>

          {!step.complete && (
            <section className="setup-guide" aria-labelledby="setup-heading">
              <div className="setup-copy">
                <h2 id="setup-heading">Finish setting up</h2>
                <p>{step.copy}</p>
              </div>
              <ol aria-label="Workspace setup">
                <li className={step.done.account ? "is-complete" : ""}>Connect an account</li>
                <li className={step.done.data ? "is-complete" : ""}>Choose data</li>
                <li className={step.done.tools ? "is-complete" : ""}>Enable tools</li>
              </ol>
              <LinkButton variant="primary" href={step.action.href}>
                {step.action.label}
              </LinkButton>
            </section>
          )}

          <section className="panel" aria-labelledby="services-heading">
            <div className="panel-head">
              <div>
                <h2 id="services-heading">Google services</h2>
                <p>Connection status and enabled assets</p>
              </div>
              <Link className="panel-link" href="/settings/data">
                Manage data access →
              </Link>
            </div>
            {rows.length === 0 ? (
              <div className="empty">
                <h2>No Google services connected yet</h2>
                <p>Connect an account to see its approved services here.</p>
                <LinkButton variant="primary" href="/settings/accounts#connect">
                  Connect Google account
                </LinkButton>
              </div>
            ) : (
              <ul className="service-table" aria-label="Google services">
                {rows.map((row) => (
                  <li key={row.service}>
                    <Link
                      className="service-row"
                      href={`/settings/data?service=${encodeURIComponent(row.service)}${row.ownerId ? "&account=" + encodeURIComponent(row.ownerId) : ""}`}
                    >
                      <span className="service-identity">
                        <ServiceMark service={row.service} />
                        <span>
                          <strong>{row.label}</strong>
                          <small>{row.category}</small>
                        </span>
                      </span>
                      <span className="service-cell">
                        {row.total ? (
                          <>
                            <strong>
                              {row.enabled} of {row.total}
                            </strong>
                            <small>assets enabled</small>
                          </>
                        ) : (
                          <>
                            <strong>—</strong>
                            <small>no assets found</small>
                          </>
                        )}
                      </span>
                      <span className="service-cell">
                        {row.accounts} {plural(row.accounts, "account", "accounts")}
                      </span>
                      <span className="service-cell status-cell">
                        {row.status === "attention" && (
                          <span className="status needs_attention">Needs attention</span>
                        )}
                        {row.status === "delayed" && (
                          <span className="status unavailable">Check delayed</span>
                        )}
                        {row.status === "empty" && (
                          <span className="service-quiet">Nothing to choose yet</span>
                        )}
                      </span>
                      <span className="row-chevron" aria-hidden="true">
                        ›
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}
    </>
  );
}

function Metric({ icon, value, label }: { icon: string; value: string; label: string }) {
  return (
    <div className="metric">
      <span className="metric-icon">
        <Icon name={icon} />
      </span>
      <div>
        <strong>{value}</strong>
        <span>{label}</span>
      </div>
    </div>
  );
}
