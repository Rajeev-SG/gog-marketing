import { useState } from "react";
import { Button, Icon } from "../../ui/index.js";

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

export function ConnectAiPage() {
  const [copied, setCopied] = useState(false);
  const url = mcpUrl();
  return (
    <>
      <header className="page-header">
        <div>
          <h1>Connect AI</h1>
          <p>Use your Google data from an AI assistant outside this app.</p>
        </div>
      </header>
      <section className="panel connect-panel" aria-labelledby="mcp-heading">
        <p className="eyebrow">Remote MCP endpoint</p>
        <h2 id="mcp-heading">Codex remote MCP endpoint</h2>
        {url ? (
          <>
            <p>
              Add this HTTPS URL as a remote MCP server in Codex. Complete the browser sign-in, then
              the assistant can use only the tools and assets enabled for your Google accounts.
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
                {copied ? "Copied" : "Copy URL"}
              </Button>
            </div>
            <p className="sr-only" role="status">
              {copied ? "MCP URL copied to the clipboard" : ""}
            </p>
          </>
        ) : (
          <p>
            The workspace address isn't configured yet, so the MCP URL is unavailable. Ask the
            workspace owner.
          </p>
        )}
        <p className="faint">
          Expanded service coverage is still being verified; this is not a public-beta readiness
          claim.
        </p>
      </section>
    </>
  );
}
