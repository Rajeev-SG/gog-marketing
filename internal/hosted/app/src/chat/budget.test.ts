import { afterAll, beforeAll, expect, it, vi } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { HostedRepository } from "../../../state/src/repository.js";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { ChatThreads, reserveChatBudget } from "./threads.js";

let mf: Miniflare;
let repo: HostedRepository;
beforeAll(async () => {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "budget",
          modules: true,
          script: "export default {};",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "budget" },
        },
      ],
    }),
  );
  const db = await mf.getD1Database("DB");
  for (const file of ["0001_initial_schema.sql", "0007_chat_threads.sql"])
    for (const sql of splitMigrationStatements(loadMigrationSql(file))) await db.prepare(sql).run();
  repo = new HostedRepository(db as unknown as D1Database);
});
afterAll(async () => {
  vi.useRealTimers();
  await mf?.dispose();
});

const utcDay = (value: number) => Math.floor(value / 86400000);
const utcMinute = (value: number) => Math.floor(value / 60000);
const utcMonth = (value: number) => new Date(value).toISOString().slice(0, 7);
const legacyAt = (now: number) => ({
  minute: utcMinute(now),
  minuteCount: 0,
  day: utcDay(now),
  dayCount: 0,
});
const quotaValue = async (tenantId: string, period: string) =>
  (await repo.getQuota(tenantId, period, "chat_model_starts"))?.value;

it("isolates monthly budgets by authoritative tenant and ignores thread identity", async () => {
  const now = Date.parse("2026-03-15T12:00:00Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
  const tenant = await repo.bootstrapTenant("budget-tenant");
  const threads = new ChatThreads(repo, tenant.id);
  const thread = (await threads.create())!;

  expect(await reserveChatBudget(repo, tenant.id, legacyAt(now))).toBe(true);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(1);

  await threads.tombstone(thread.id);
  await threads.create();
  expect(await reserveChatBudget(repo, tenant.id, legacyAt(now))).toBe(true);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(2);

  const other = await repo.bootstrapTenant("budget-tenant-other");
  expect(await reserveChatBudget(repo, other.id, legacyAt(now))).toBe(true);
  expect(await quotaValue(other.id, "chat-month:2026-03")).toBe(1);
});

it("carries legacy daily consumption into the month without repeated inflation", async () => {
  const now = Date.parse("2026-03-16T13:00:00Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
  const tenant = await repo.bootstrapTenant("budget-legacy");
  const legacy = { ...legacyAt(now), dayCount: 99 };

  expect(await reserveChatBudget(repo, tenant.id, legacy)).toBe(true);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(100);
  expect(await reserveChatBudget(repo, tenant.id)).toBe(false);

  // A later snapshot of the same legacy day cannot add the 99 again.
  expect(await reserveChatBudget(repo, tenant.id, legacy)).toBe(false);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(100);
});

it("resets day and month accounting on their UTC boundaries", async () => {
  const start = Date.parse("2026-03-31T23:59:30Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(start);
  const tenant = await repo.bootstrapTenant("budget-boundary");
  expect(await reserveChatBudget(repo, tenant.id, legacyAt(start))).toBe(true);
  expect(await quotaValue(tenant.id, "chat-day:" + utcDay(start))).toBe(1);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(1);

  const nextMinute = Date.parse("2026-04-01T00:00:30Z");
  vi.setSystemTime(nextMinute);
  expect(await reserveChatBudget(repo, tenant.id, legacyAt(nextMinute))).toBe(true);
  expect(await quotaValue(tenant.id, "chat-day:" + utcDay(nextMinute))).toBe(1);
  expect(await quotaValue(tenant.id, "chat-month:" + utcMonth(nextMinute))).toBe(1);
});

it("never admits more than fixed limits under concurrent reservation", async () => {
  const now = Date.parse("2026-03-17T14:00:00Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
  const tenant = await repo.bootstrapTenant("budget-race");
  const minutePeriod = "chat-minute:" + utcMinute(now);
  const dayPeriod = "chat-day:" + utcDay(now);

  // Make minute/day the binding constraints while exercising one atomic
  // reservation path from concurrent callers.
  await repo.incrementQuota(tenant.id, minutePeriod, "chat_model_starts", 9);
  await repo.incrementQuota(tenant.id, dayPeriod, "chat_model_starts", 99);
  await repo.incrementQuota(tenant.id, "chat-month:2026-03", "chat_model_starts", 999);
  const results = await Promise.all(
    Array.from({ length: 8 }, () => reserveChatBudget(repo, tenant.id)),
  );

  expect(results.filter(Boolean)).toHaveLength(1);
  expect(await quotaValue(tenant.id, minutePeriod)).toBe(10);
  expect(await quotaValue(tenant.id, dayPeriod)).toBe(100);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(1000);
});

it("fails closed when the monthly ceiling is exhausted", async () => {
  const now = Date.parse("2026-03-18T15:00:00Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
  const tenant = await repo.bootstrapTenant("budget-month-limit");
  await repo.incrementQuota(tenant.id, "chat-month:2026-03", "chat_model_starts", 1000);

  expect(await reserveChatBudget(repo, tenant.id)).toBe(false);
  expect(await quotaValue(tenant.id, "chat-day:" + utcDay(now))).toBe(1);
});

it("atomically enforces the monthly ceiling when minute and day budgets remain available", async () => {
  const now = Date.parse("2026-03-19T15:00:00Z");
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
  const tenant = await repo.bootstrapTenant("budget-month-race");
  await repo.incrementQuota(tenant.id, "chat-month:2026-03", "chat_model_starts", 999);
  const results = await Promise.all(
    Array.from({ length: 8 }, () => reserveChatBudget(repo, tenant.id)),
  );
  expect(results.filter(Boolean)).toHaveLength(1);
  expect(await quotaValue(tenant.id, "chat-month:2026-03")).toBe(1000);
  expect(await quotaValue(tenant.id, "chat-day:" + utcDay(now))).toBe(8);
});
