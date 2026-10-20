import { afterAll, beforeAll, expect, it } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { HostedRepository } from "../../../state/src/repository.js";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import {
  DEFAULT_MODEL_REQUEST_LIMITS,
  reserveModelRequest,
  type ModelRequestLimits,
} from "./model-budgets.js";

let mf: Miniflare;
let repo: HostedRepository;
let db: D1Database;

beforeAll(async () => {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "model-budgets",
          modules: true,
          script: "export default {};",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "model-budgets" },
        },
      ],
    }),
  );
  db = (await mf.getD1Database("DB")) as unknown as D1Database;
  for (const file of ["0001_initial_schema.sql", "0003_rate_limit_counters.sql"]) {
    for (const sql of splitMigrationStatements(loadMigrationSql(file))) {
      await db.prepare(sql).run();
    }
  }
  repo = new HostedRepository(db as unknown as D1Database);
});

afterAll(async () => {
  await mf?.dispose();
});

const tenantScope = (tenantId: string) => `gog-chat:tenant:${tenantId}`;
const counter = async (scopeKey: string, period: string) => {
  const row = await db
    .prepare(
      "SELECT value FROM hosted_rate_limit_counters WHERE scope_kind = 'tenant' AND scope_key = ?1 AND period = ?2",
    )
    .bind(scopeKey, period)
    .first<{ value: number }>();
  return row ? Number(row.value) : 0;
};

const reservation = (overrides: Partial<Parameters<typeof reserveModelRequest>[2]> = {}) => ({
  inputTokens: 1,
  outputTokens: 1,
  costMicroUsd: 0,
  paid: false,
  ...overrides,
});

it("enforces tenant and global request limits under 100 concurrent reservations", async () => {
  const now = Date.parse("2026-03-20T12:00:00Z");
  const limits: ModelRequestLimits = {
    ...DEFAULT_MODEL_REQUEST_LIMITS,
    tenantMinute: 5,
    tenantDay: 5,
    tenantMonth: 5,
    globalMinute: 30,
    globalDay: 30,
  };
  const tenants = await Promise.all(
    Array.from({ length: 10 }, (_, index) => repo.bootstrapTenant(`model-budgets-race-${index}`)),
  );
  const results = await Promise.all(
    tenants.flatMap((tenant) =>
      Array.from({ length: 10 }, () =>
        reserveModelRequest(repo, tenant.id, reservation(), limits, now),
      ),
    ),
  );

  expect(results.filter(Boolean).length).toBeLessThanOrEqual(30);
  for (const tenant of tenants) {
    expect(
      await counter(tenantScope(tenant.id), `model-requests-minute:${Math.floor(now / 60_000)}`),
    ).toBeLessThanOrEqual(5);
  }
  expect(
    await counter("gog-chat:global", `model-requests-minute:${Math.floor(now / 60_000)}`),
  ).toBe(30);
}, 20_000);

it("caps tenant and global tokens and cost under 100 concurrent reservations", async () => {
  const now = Date.parse("2026-04-21T12:00:00Z");
  const limits: ModelRequestLimits = {
    ...DEFAULT_MODEL_REQUEST_LIMITS,
    tenantMinute: 100,
    tenantDay: 100,
    tenantMonth: 100,
    globalMinute: 100,
    globalDay: 100,
    tenantDayTokens: 25,
    globalDayTokens: 25,
    tenantMonthCostMicroUsd: 33,
    globalMonthCostMicroUsd: 33,
  };
  const tenant = await repo.bootstrapTenant("model-budgets-token-cost-race");
  const input = reservation({ inputTokens: 4, outputTokens: 3, costMicroUsd: 11, paid: true });
  const results = await Promise.all(
    Array.from({ length: 100 }, () => reserveModelRequest(repo, tenant.id, input, limits, now)),
  );

  expect(results.filter(Boolean).length).toBeLessThanOrEqual(3);
  const day = String(Math.floor(now / 86_400_000));
  expect(await counter(tenantScope(tenant.id), `model-tokens-day:${day}`)).toBe(21);
  expect(await counter("gog-chat:global", `model-tokens-day:${day}`)).toBe(21);
  expect(await counter(tenantScope(tenant.id), `model-cost-micro-usd-month:2026-04`)).toBe(33);
  expect(await counter("gog-chat:global", `model-cost-micro-usd-month:2026-04`)).toBe(33);
}, 20_000);

it("resets UTC minute, day, and tenant month windows", async () => {
  const before = Date.parse("2026-05-31T23:59:30Z");
  const after = Date.parse("2026-06-01T00:00:30Z");
  const limits: ModelRequestLimits = {
    ...DEFAULT_MODEL_REQUEST_LIMITS,
    tenantMinute: 1,
    tenantDay: 1,
    tenantMonth: 1,
    globalMinute: 1,
    globalDay: 1,
  };
  const tenant = await repo.bootstrapTenant("model-budgets-utc-reset");

  expect(await reserveModelRequest(repo, tenant.id, reservation(), limits, before)).toBe(true);
  expect(await reserveModelRequest(repo, tenant.id, reservation(), limits, before)).toBe(false);
  expect(await reserveModelRequest(repo, tenant.id, reservation(), limits, after)).toBe(true);
});

it("isolates model request budgets by internal tenant", async () => {
  const now = Date.parse("2026-07-22T12:00:00Z");
  const limits: ModelRequestLimits = {
    ...DEFAULT_MODEL_REQUEST_LIMITS,
    tenantMinute: 1,
    tenantDay: 1,
    tenantMonth: 1,
  };
  const first = await repo.bootstrapTenant("model-budgets-tenant-a");
  const second = await repo.bootstrapTenant("model-budgets-tenant-b");

  expect(await reserveModelRequest(repo, first.id, reservation(), limits, now)).toBe(true);
  expect(await reserveModelRequest(repo, second.id, reservation(), limits, now)).toBe(true);
  expect(await reserveModelRequest(repo, first.id, reservation(), limits, now)).toBe(false);
  expect(await reserveModelRequest(repo, second.id, reservation(), limits, now)).toBe(false);
});

it("allows the zero-cost default without paid accounting and rejects paid costs by default", async () => {
  const now = Date.parse("2026-08-23T12:00:00Z");
  const tenant = await repo.bootstrapTenant("model-budgets-zero-paid");
  const month = new Date(now).toISOString().slice(0, 7);

  expect(
    await reserveModelRequest(
      repo,
      tenant.id,
      reservation({ inputTokens: 2, outputTokens: 3 }),
      undefined,
      now,
    ),
  ).toBe(true);
  expect(await counter(tenantScope(tenant.id), `model-cost-micro-usd-month:${month}`)).toBe(0);
  expect(await counter("gog-chat:global", `model-cost-micro-usd-month:${month}`)).toBe(0);

  expect(
    await reserveModelRequest(
      repo,
      tenant.id,
      reservation({ costMicroUsd: 1, paid: true }),
      undefined,
      now,
    ),
  ).toBe(false);
  expect(await counter(tenantScope(tenant.id), `model-cost-micro-usd-month:${month}`)).toBe(0);
});

it("fails closed on invalid tenant IDs, reservations, limits, and timestamps", async () => {
  const now = Date.parse("2026-09-24T12:00:00Z");
  const tenant = await repo.bootstrapTenant("model-budgets-invalid-input");
  const beforeMinute = `model-requests-minute:${Math.floor(now / 60_000)}`;
  const invalidReservations = [
    reservation({ inputTokens: 1.5 }),
    reservation({ inputTokens: -1 }),
    reservation({ outputTokens: Number.MAX_SAFE_INTEGER + 1 }),
    reservation({ costMicroUsd: 1 }),
    reservation({ costMicroUsd: 0, paid: true }),
    reservation({ costMicroUsd: -1, paid: true }),
  ];
  const invalidLimits: ModelRequestLimits[] = [
    { ...DEFAULT_MODEL_REQUEST_LIMITS, tenantMinute: 1.5 },
    { ...DEFAULT_MODEL_REQUEST_LIMITS, tenantDay: -1 },
    { ...DEFAULT_MODEL_REQUEST_LIMITS, tenantDayTokens: Number.MAX_SAFE_INTEGER + 1 },
  ];

  for (const invalid of invalidReservations) {
    expect(await reserveModelRequest(repo, tenant.id, invalid, undefined, now)).toBe(false);
  }
  for (const limits of invalidLimits) {
    expect(await reserveModelRequest(repo, tenant.id, reservation(), limits, now)).toBe(false);
  }
  expect(await reserveModelRequest(repo, "not-a-tenant-id", reservation(), undefined, now)).toBe(
    false,
  );
  expect(await reserveModelRequest(repo, tenant.id, reservation(), undefined, 1.5)).toBe(false);
  expect(await counter(tenantScope(tenant.id), beforeMinute)).toBe(0);
  expect(await counter("gog-chat:global", beforeMinute)).toBe(0);
});

it("does not block zero-cost routing after paid budgets are exhausted or disabled", async () => {
  const now = Date.parse("2026-10-20T12:00:00Z");
  const tenant = await repo.bootstrapTenant("model-budget-free-after-paid");
  const paidLimits = {
    ...DEFAULT_MODEL_REQUEST_LIMITS,
    tenantMonthCostMicroUsd: 1,
    globalMonthCostMicroUsd: 1,
  };
  expect(
    await reserveModelRequest(
      repo,
      tenant.id,
      reservation({ costMicroUsd: 1, paid: true }),
      paidLimits,
      now,
    ),
  ).toBe(true);
  expect(
    await reserveModelRequest(repo, tenant.id, reservation(), DEFAULT_MODEL_REQUEST_LIMITS, now),
  ).toBe(true);
  expect(
    await reserveModelRequest(
      repo,
      tenant.id,
      reservation({ costMicroUsd: 1, paid: true }),
      paidLimits,
      now,
    ),
  ).toBe(false);
});
