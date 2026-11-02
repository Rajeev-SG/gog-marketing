import { clerkScriptTags, type HtmlConfig } from "./html.js";
import { renderAccessState } from "./access-pages.js";

/**
 * The signed-in workspace (Home, Google data, Access, Connect AI) is the built
 * React app in dist/client/app.html. The server injects Clerk and the canonical
 * MCP URL; it never derives either from request headers.
 */
export async function renderWorkspacePage(
  assets: Fetcher | undefined,
  requestUrl: URL,
  htmlConfig: HtmlConfig,
  headers: Headers,
): Promise<Response> {
  const asset = assets
    ? await assets.fetch(
        new Request(new URL("/app.html", requestUrl), { headers: { Accept: "text/html" } }),
      )
    : null;
  headers.set("Content-Type", "text/html; charset=utf-8");
  headers.set("Cache-Control", "no-store");
  if (!asset?.ok)
    return new Response(renderAccessState("unavailable", "/"), { status: 503, headers });
  const mcpUrl = htmlConfig.mcpOrigin ? new URL("/mcp", htmlConfig.mcpOrigin).toString() : "";
  const config = JSON.stringify({ mcpUrl }).replaceAll("<", "\\u003c");
  const bootstrap =
    clerkScriptTags(htmlConfig) +
    '<script>window.addEventListener("load",async function(){await window.Clerk.load({ui:{ClerkUI:window.__internal_ClerkUICtor}});});</script>' +
    `<script type="application/json" id="gog-config">${config}</script>`;
  const html = (await asset.text()).replace("</head>", bootstrap + "</head>");
  return new Response(html, { status: 200, headers });
}
