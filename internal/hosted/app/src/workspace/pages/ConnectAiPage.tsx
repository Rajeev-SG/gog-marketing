import { useState } from "react";
import { Button, Disclosure, Icon } from "../../ui/index.js";

/** The server injects the canonical MCP URL; the browser never derives it from its own host. */
function mcpUrl(): string {
  try {
    const raw = document.getElementById("gog-config")?.textContent ?? "{}";
    const value = (JSON.parse(raw) as { mcpUrl?: unknown }).mcpUrl;
    return typeof value === "string" ? value : "";
  } catch {
    return "";
  }
}

const APPS = [
  {
    id: "chatgpt",
    name: "ChatGPT",
    steps: [
      "In ChatGPT, open Settings › Apps & Connectors and turn on Developer mode (Advanced settings).",
      "Choose Create, name it gog-marketing, and paste the address above as the MCP server URL. Leave authentication as OAuth.",
      "Sign in when ChatGPT opens gog-marketing, then pick the connector from the + menu in a chat.",
    ],
  },
  {
    id: "claude",
    name: "Claude",
    steps: [
      "In Claude, open Settings › Connectors and choose Add custom connector.",
      "Name it gog-marketing and paste the address above.",
      "Choose Connect and sign in. Turn the connector on from the tools menu in a chat.",
    ],
  },
  {
    id: "codex",
    name: "Codex",
    steps: [
      "Add a remote MCP server with the address above, for example: codex mcp add gog-marketing --url <address>.",
      "Run codex mcp login gog-marketing and sign in in the browser window that opens.",
    ],
  },
];

/** Settings › Connect AI apps: use your Google data from AI apps outside this one. */
export function ConnectAiPage() {
  const [copied, setCopied] = useState(false);
  const url = mcpUrl();
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Connect AI apps</h1>
          <p>
            Use your Google data from ChatGPT, Claude, Codex or another app that supports remote MCP
            servers.
          </p>
        </div>
      </header>
      <section className="panel connect-panel" aria-labelledby="mcp-heading">
        <h2 id="mcp-heading">Your connection address</h2>
        {url ? (
          <>
            <p>
              Paste this address into the AI app, then sign in with this account. The app can use
              only the data and tools you enabled in Settings, and it asks before any change.
            </p>
            <div className="ai-panel">
              <code id="mcp-url">{url}</code>
              <Button
                onPress={async () => {
                  await navigator.clipboard.writeText(url);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 1800);
                }}
              >
                <Icon name={copied ? "check" : "copy"} />
                {copied ? "Copied" : "Copy address"}
              </Button>
            </div>
            <p className="sr-only" role="status">
              {copied ? "Address copied to the clipboard" : ""}
            </p>
          </>
        ) : (
          <p>
            The workspace address isn’t configured yet, so the connection address is unavailable.
            Ask the workspace owner.
          </p>
        )}
      </section>
      <section className="panel" aria-labelledby="apps-heading">
        <div className="panel-head">
          <div>
            <h2 id="apps-heading">Set up an app</h2>
            <p>Menu names can change between app versions.</p>
          </div>
        </div>
        <div className="tool-panel">
          {APPS.map((app) => (
            <Disclosure key={app.id} title={app.name} className="app-setup">
              <ol className="app-steps">
                {app.steps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            </Disclosure>
          ))}
        </div>
      </section>
    </>
  );
}
