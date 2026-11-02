import { useState } from "react";
import { CONSENT_GROUPS, consentServiceIds } from "../../consent.js";
import { HOSTED_SERVICES } from "../../scopes.js";
import { Button, Checkbox, Icon, LinkButton, Notice } from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { useLocation } from "../router.js";
import { ServiceMark } from "../ServiceMark.js";
import { accountName, requestConnection, serviceKey } from "../model.js";

export function AccessPage() {
  const ws = useWorkspace();
  const url = useLocation();
  const target = ws.connections.find((item) => item.id === url.searchParams.get("update"));

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Access</h1>
          <p>Which Google services each account shares, and how to add or change that.</p>
        </div>
      </header>

      <section className="access-strip" aria-labelledby="how-heading">
        <Icon name="shield" />
        <div>
          <h2 id="how-heading">How access works</h2>
          <ol className="access-steps-inline">
            <li>
              <strong>Connect.</strong> You choose which Google services an account shares. Reads
              and changes are separate choices.
            </li>
            <li>
              <strong>Choose data.</strong> AI tools see only the assets you enable on Google data.
              New assets start disabled.
            </li>
            <li>
              <strong>Choose tools.</strong> Each tool is enabled per account. Every Google change
              is shown to you before it runs.
            </li>
          </ol>
        </div>
      </section>

      {ws.connections.length > 0 && (
        <section className="panel" aria-labelledby="granted-heading">
          <div className="panel-head">
            <div>
              <h2 id="granted-heading">Shared services by account</h2>
              <p>What each connected account allows. Change it with Update Google permissions.</p>
            </div>
          </div>
          <ul className="granted-list">
            {ws.connections.map((connection) => (
              <li key={connection.id} className="granted-account">
                <div className="granted-head">
                  <h3>{accountName(connection)}</h3>
                  <LinkButton href={`/access?update=${encodeURIComponent(connection.id)}#connect`}>
                    Update Google permissions
                  </LinkButton>
                </div>
                <ul
                  className="granted-services"
                  aria-label={`Services shared by ${accountName(connection)}`}
                >
                  {(connection.services ?? []).map((name) => (
                    <li key={name}>
                      <ServiceMark service={serviceKey(name)} />
                      {name}
                    </li>
                  ))}
                  {!(connection.services ?? []).length && (
                    <li className="muted">No services shared</li>
                  )}
                </ul>
              </li>
            ))}
          </ul>
        </section>
      )}

      <ConnectForm
        key={target?.id ?? "new"}
        targetId={target?.id ?? null}
        targetName={target ? accountName(target) : ""}
      />
    </>
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
          <p className="eyebrow">{targetId ? "Update access" : "Add access"}</p>
          <h2 id="connect-heading">
            {targetId ? `Update Google permissions for ${targetName}` : "Connect a Google account"}
          </h2>
          {targetId ? (
            <Notice tone="info">
              Updating permissions for {targetName}. Existing approvals are preserved; select
              additional services, then continue to Google.{" "}
              <LinkButton href="/access#connect">Connect a different account instead</LinkButton>
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
            {all.length} service choices are shown. Read and write consent controls Google data
            access only—it does not enable any assets or tool permissions.
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
              {targetId ? "Update Google permissions" : "Connect Google account"}
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
