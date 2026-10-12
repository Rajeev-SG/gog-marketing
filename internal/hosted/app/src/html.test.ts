import { describe, expect, it } from "vitest";

import { renderHome } from "./html.js";
import { PRODUCT_CSS } from "./product-css.generated.js";

const config = {
  publishableKey: "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA",
  authorizedParties: ["https://gog-marketing.rajeev-sgill.workers.dev"],
  clerkDomain: "oriented-wombat-2813.clerk.accounts.dev",
  mcpOrigin: "https://gog-marketing.rajeev-sgill.workers.dev",
};

describe("hosted marketer UI reference states (#50)", () => {
  const html = renderHome(config);

  it("uses the shared product design layer and persistent application shell", () => {
    expect(html).toContain(PRODUCT_CSS);
    expect(html).toContain('class="app-shell"');
    expect(html).toContain('class="sidebar"');
    expect(html).toContain('class="bottom-nav"');
    expect(html).toContain("Your Google marketing data");
  });

  it("represents loading, partial failure, completion and duplicate feedback", () => {
    expect(html).toContain('id="loading-state"');
    expect(html).toContain("Finding your Google marketing assets…");
    expect(html).toContain("Some Google services couldn’t be checked");
    expect(html).toContain("Your existing selections are unchanged and remain available.");
    expect(html).toContain('id="state-banner"');
    expect(html).toContain("function selectedCountFor(connection)");
    expect(html).toContain("completion.dataset.connectionId");
    expect(html).toContain('id="completion-state"');
    expect(html).toContain("Your Google data is ready");
    expect(html).toContain("Already connected — this Google account is already available.");
    expect(html).toContain("Choose the original Google account for this connection");
  });

  it("keeps healthy operations contextual and uses marketer-facing status copy", () => {
    expect(html).toContain("menu.className = 'context-menu'");
    expect(html).not.toContain("Discovery ready");
    expect(html).not.toContain("Discovery unavailable");
    expect(html).not.toContain("action('Reconnect'");
    expect(html).not.toContain("action('Refresh'");
    expect(html).not.toContain("action('Discover'");
  });

  it("includes responsive asset and mobile navigation states", () => {
    expect(html).toContain('id="asset-workspace"');
    expect(html).toContain('id="asset-search"');
    expect(html).toContain("@media (max-width: 680px)");
    expect(html).toContain("@media (max-width: 900px)");
  });
});
