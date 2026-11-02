import { describe, expect, it } from "vitest";

import {
  enableVisibleReadTools,
  filteredToolCatalog,
  readableToolLabel,
  toolGrantPayload,
  toolGroups,
  type Tool,
  type ToolChoice,
} from "./workspace/model.js";

const tool = (name: string, service: string, write: boolean, description: string): Tool => ({
  name,
  service,
  description,
  write,
  enabled: false,
  allowDestructive: false,
});
const toolCatalog = [
  tool("docs_read", "docs", false, "Read a Google Doc."),
  tool("docs_write", "docs", true, "Write text to a Google Doc."),
  tool("sheets_read_range", "sheets", false, "Read a Sheets range."),
];

describe("exact-tool permissions rules (#107)", () => {
  it("searches readable names and machine IDs while filters only access classes", () => {
    expect(readableToolLabel(toolCatalog[1]!)).toBe("Google Docs · Write");
    expect(filteredToolCatalog(toolCatalog, "Google Docs · Write", "all")).toEqual([
      toolCatalog[1],
    ]);
    expect(filteredToolCatalog(toolCatalog, "docs_write", "all")).toEqual([toolCatalog[1]]);
    expect(filteredToolCatalog(toolCatalog, "", "reads").map((item) => item.name)).toEqual([
      "docs_read",
      "sheets_read_range",
    ]);
    expect(filteredToolCatalog(toolCatalog, "", "changes").map((item) => item.name)).toEqual([
      "docs_write",
    ]);
  });

  it("bulk-enables only visible read-only tools and preserves every write choice", () => {
    const choices = new Map<string, ToolChoice>([
      ["docs_read", { toolName: "docs_read", enabled: false, allowDestructive: false }],
      [
        "sheets_read_range",
        { toolName: "sheets_read_range", enabled: true, allowDestructive: false },
      ],
      ["docs_write", { toolName: "docs_write", enabled: true, allowDestructive: true }],
    ]);
    expect(enableVisibleReadTools(choices, toolCatalog, "", "all")).toBe(1);
    expect(choices.get("docs_read")).toEqual({
      toolName: "docs_read",
      enabled: true,
      allowDestructive: false,
    });
    expect(choices.get("docs_write")).toEqual({
      toolName: "docs_write",
      enabled: true,
      allowDestructive: true,
    });
  });

  it("bulk-enable respects the current search and never touches a hidden or write tool", () => {
    const choices = new Map<string, ToolChoice>(
      toolCatalog.map((item) => [
        item.name,
        { toolName: item.name, enabled: false, allowDestructive: false },
      ]),
    );
    expect(enableVisibleReadTools(choices, toolCatalog, "sheets", "all")).toBe(1);
    expect(choices.get("docs_read")!.enabled).toBe(false);
    expect(choices.get("docs_write")!.enabled).toBe(false);
  });

  it("keeps the save payload exactly toolName, enabled and allowDestructive", () => {
    const choices = new Map<string, ToolChoice>([
      [
        "docs_write",
        {
          toolName: "docs_write",
          enabled: true,
          allowDestructive: false,
          extra: true,
        } as ToolChoice,
      ],
    ]);
    expect(toolGrantPayload(choices)).toEqual([
      { toolName: "docs_write", enabled: true, allowDestructive: false },
    ]);
  });

  it("groups tools by service, Marketing before Workspace", () => {
    const groups = toolGroups([
      ...toolCatalog,
      tool("analytics_run_report", "analytics", false, "Run a report."),
    ]);
    expect(groups.map((group) => group.service)).toEqual(["analytics", "docs", "sheets"]);
  });
});
