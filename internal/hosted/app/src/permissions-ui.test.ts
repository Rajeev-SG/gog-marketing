import vm from "node:vm";

import { describe, expect, it } from "vitest";

import { renderHome } from "./html.js";

const config = {
  publishableKey: "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA",
  authorizedParties: ["https://gog-marketing.rajeev-sgill.workers.dev"],
  clerkDomain: "oriented-wombat-2813.clerk.accounts.dev",
  mcpOrigin: "https://gog-marketing.rajeev-sgill.workers.dev",
};

const toolCatalog = [
  {
    name: "docs_read",
    service: "docs",
    description: "Read a Google Doc.",
    write: false,
  },
  {
    name: "docs_write",
    service: "docs",
    description: "Write text to a Google Doc.",
    write: true,
  },
  {
    name: "sheets_read_range",
    service: "sheets",
    description: "Read a Sheets range.",
    write: false,
  },
];

describe("progressive exact-tool permissions UI (#107)", () => {
  const page = renderHome(config);

  it("provides account context, states, grouping, search, filters and retry markup", () => {
    expect(page).toContain('id="tool-account"');
    expect(page).toContain('id="tool-search"');
    expect(page).toContain('aria-label="Search tools by name or machine ID"');
    expect(page).toContain('data-tool-filter="all"');
    expect(page).toContain('data-tool-filter="reads"');
    expect(page).toContain('data-tool-filter="changes"');
    expect(page).toContain('id="tool-loading"');
    expect(page).toContain('id="tool-error"');
    expect(page).toContain('id="retry-tools"');
    expect(page).toContain('id="tool-empty"');
    expect(page).toContain('id="tool-search-zero"');
    expect(page).toContain("function toolGroups(visibleTools)");
    expect(page).toContain("className = 'tool-group'");
    expect(page).toContain("Show technical description");
  });

  it("scopes bulk reads narrowly and states the write/destructive guarantee", () => {
    expect(page).toContain('id="enable-tool-reads"');
    expect(page).toContain("Enable read-only tools in this list");
    expect(page).toContain(
      "It never enables write tools or destructive operations, and it leaves every write and destructive choice unchanged.",
    );
    expect(page).toContain("function enableVisibleReadTools(choices, catalog, query, filter)");
  });

  it("keeps explicit write/destructive confirmation, race and dirty-change protections", () => {
    expect(page).toContain("window.confirm('Allow ' + tool.name + ' to change Google data for '");
    expect(page).toContain(
      "window.confirm('Allow destructive replacement/deletion for ' + tool.name + '? This is a separate account-specific permission.'",
    );
    expect(page).toContain(
      "toolChoicesDirty && !window.confirm('Discard unsaved tool permission changes for this account?')",
    );
    expect(page).toContain(
      "if (!selectedAccount() || selectedAccount().id !== connectionId) return;",
    );
    expect(page).toContain("if (selectedAccount() && selectedAccount().id === connectionId)");
  });

  it("keeps the save payload exactly toolName, enabled and allowDestructive", () => {
    expect(page).toContain(
      "return { toolName: choice.toolName, enabled: choice.enabled, allowDestructive: choice.allowDestructive };",
    );
    expect(page).not.toContain("allowBillable: choice.allowBillable");
  });
});

describe("executed exact-tool permissions helpers (#107)", () => {
  function executeHelpers() {
    const page = renderHome(config);
    const start = page.indexOf("var toolChoices = new Map();");
    const end = page.indexOf("async function jsonFetch");
    expect(start).toBeGreaterThan(-1);
    expect(end).toBeGreaterThan(start);

    const sandbox = {
      label: (service: string) =>
        ({ docs: "Google Docs", sheets: "Google Sheets" })[service] ?? service,
      category: () => "Workspace",
      document: { createElement: () => ({}) },
      el: () => ({}),
      selectedAccount: () => null,
      fixtures: toolCatalog,
    };
    const context = vm.createContext(sandbox);
    vm.runInNewContext(page.slice(start, end), context, {
      filename: "html.ts renderHome tool permissions helpers",
    });
    vm.runInContext("toolCatalog = fixtures", context);

    return {
      evaluate: (expression: string) => vm.runInContext(expression, context, { timeout: 1000 }),
    };
  }

  it("searches readable names and machine IDs while filters only access classes", () => {
    const run = executeHelpers();
    const readableLabel = run.evaluate("readableToolLabel(toolCatalog[1])") as string;
    expect(readableLabel).toBe("Google Docs · Write");
    expect(run.evaluate("filteredToolCatalog(toolCatalog, 'Google Docs · Write', 'all')")).toEqual([
      toolCatalog[1],
    ]);
    expect(run.evaluate("filteredToolCatalog(toolCatalog, 'docs_write', 'all')")).toEqual([
      toolCatalog[1],
    ]);
    expect(
      run.evaluate(
        "filteredToolCatalog(toolCatalog, '', 'reads').map(function (tool) { return tool.name; })",
      ),
    ).toEqual(["docs_read", "sheets_read_range"]);
    expect(
      run.evaluate(
        "filteredToolCatalog(toolCatalog, '', 'changes').map(function (tool) { return tool.name; })",
      ),
    ).toEqual(["docs_write"]);
  });

  it("bulk-enables only visible read-only tools and preserves every write choice", () => {
    const run = executeHelpers();
    const result = run.evaluate(`
      (function () {
        var choices = new Map([
          ['docs_read', { toolName: 'docs_read', enabled: false, allowDestructive: false }],
          ['sheets_read_range', { toolName: 'sheets_read_range', enabled: true, allowDestructive: false }],
          ['docs_write', { toolName: 'docs_write', enabled: true, allowDestructive: true }]
        ]);
        var changed = enableVisibleReadTools(choices, toolCatalog, '', 'all');
        return {
          changed: changed,
          docsRead: choices.get('docs_read'),
          docsWrite: choices.get('docs_write')
        };
      })()
    `) as {
      changed: number;
      docsRead: { toolName: string; enabled: boolean; allowDestructive: boolean };
      docsWrite: { toolName: string; enabled: boolean; allowDestructive: boolean };
    };
    expect(result.changed).toBe(1);
    expect(result.docsRead).toEqual({
      toolName: "docs_read",
      enabled: true,
      allowDestructive: false,
    });
    expect(result.docsWrite).toEqual({
      toolName: "docs_write",
      enabled: true,
      allowDestructive: true,
    });
  });
});
