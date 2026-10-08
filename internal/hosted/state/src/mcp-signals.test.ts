import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { applyMigrationSql, createTestDatabase, type SQLiteD1Adapter } from "./sqlite-adapter.js";
import { loadMigrationSql } from "./dev-migrations.js";
import { HostedRepository } from "./repository.js";
import type { McpObservableSignal } from "./types.js";

describe("tenant-independent MCP signal counters", () => {
  let db: SQLiteD1Adapter;
  let repo: HostedRepository;

  beforeEach(() => {
    db = createTestDatabase();
    applyMigrationSql(db, loadMigrationSql("0004_mcp_signal_counters.sql"));
    repo = new HostedRepository(db);
  });

  afterEach(() => {
    db.close();
  });

  it("increments each secret-free signal independently by period", async () => {
    expect(await repo.incrementMcpSignal("auth_invalid_bearer", "2026-10-08")).toBe(1);
    expect(await repo.incrementMcpSignal("auth_invalid_bearer", "2026-10-08")).toBe(2);
    expect(await repo.incrementMcpSignal("auth_invalid_bearer", "2026-10-09")).toBe(1);
    expect(await repo.incrementMcpSignal("mcp_parse_error", "2026-10-08")).toBe(1);

    expect(await repo.getMcpSignal("auth_invalid_bearer", "2026-10-08")).toMatchObject({
      signal: "auth_invalid_bearer",
      period: "2026-10-08",
      value: 2,
    });
    expect(await repo.getMcpSignal("mcp_parse_error", "2026-10-08")).toMatchObject({ value: 1 });
    expect(await repo.getMcpSignal("mcp_method_not_found", "2026-10-08")).toBeNull();
  });

  it("rejects unknown signal labels and invalid increments", async () => {
    await expect(
      repo.incrementMcpSignal("request_body" as McpObservableSignal, "2026-10-08"),
    ).rejects.toThrow(/unsupported signal/i);
    await expect(repo.incrementMcpSignal("mcp_internal_error", "2026-10-08", 0)).rejects.toThrow(
      /positive integer/i,
    );
  });
});
