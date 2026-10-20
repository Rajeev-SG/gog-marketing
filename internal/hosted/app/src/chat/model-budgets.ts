import type { HostedRepository } from "../../../state/src/repository.js";

export interface ModelRequestLimits {
  tenantMinute: number;
  tenantDay: number;
  tenantMonth: number;
  globalMinute: number;
  globalDay: number;
  tenantDayTokens: number;
  globalDayTokens: number;
  tenantMonthCostMicroUsd: number;
  globalMonthCostMicroUsd: number;
}

export const DEFAULT_MODEL_REQUEST_LIMITS: ModelRequestLimits = {
  tenantMinute: 10,
  tenantDay: 100,
  tenantMonth: 1000,
  globalMinute: 20,
  globalDay: 50,
  tenantDayTokens: 2_000_000,
  globalDayTokens: 10_000_000,
  tenantMonthCostMicroUsd: 0,
  globalMonthCostMicroUsd: 0,
};

export interface ModelRequestReservation {
  inputTokens: number;
  outputTokens: number;
  costMicroUsd: number;
  paid: boolean;
}

const GLOBAL_SCOPE_KEY = "gog-chat:global";
const TENANT_SCOPE_PREFIX = "gog-chat:tenant:";
const TENANT_ID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function safeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function utcMinute(now: number): string {
  return String(Math.floor(now / 60_000));
}

function utcDay(now: number): string {
  return String(Math.floor(now / 86_400_000));
}

function utcMonth(now: number): string {
  const date = new Date(now);
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  return `${date.getUTCFullYear()}-${month}`;
}

function conditionalUpsert(
  repo: HostedRepository,
  scopeKey: string,
  period: string,
  delta: number,
  cap: number,
  timestamp: string,
) {
  return repo.db
    .prepare(
      `INSERT INTO hosted_rate_limit_counters
         (scope_kind, scope_key, period, value, updated_at)
       SELECT 'tenant', ?1, ?2, ?3, ?4 WHERE ?3 <= ?5
       ON CONFLICT (scope_kind, scope_key, period) DO UPDATE SET
         value = hosted_rate_limit_counters.value + excluded.value,
         updated_at = excluded.updated_at
       WHERE hosted_rate_limit_counters.value + excluded.value <= ?5
       RETURNING 1 AS allowed`,
    )
    .bind(scopeKey, period, delta, timestamp, cap);
}

export async function reserveModelRequest(
  repo: HostedRepository,
  tenantId: string,
  reservation: ModelRequestReservation,
  limits: ModelRequestLimits = DEFAULT_MODEL_REQUEST_LIMITS,
  now: number = Date.now(),
): Promise<boolean> {
  if (
    typeof tenantId !== "string" ||
    !TENANT_ID_PATTERN.test(tenantId) ||
    !safeInteger(now) ||
    !Number.isFinite(new Date(now).getTime()) ||
    reservation === null ||
    typeof reservation !== "object" ||
    typeof reservation.paid !== "boolean" ||
    !safeInteger(reservation.inputTokens) ||
    !safeInteger(reservation.outputTokens) ||
    !safeInteger(reservation.costMicroUsd)
  ) {
    return false;
  }

  const limitsAreIntegers =
    safeInteger(limits.tenantMinute) &&
    safeInteger(limits.tenantDay) &&
    safeInteger(limits.tenantMonth) &&
    safeInteger(limits.globalMinute) &&
    safeInteger(limits.globalDay) &&
    safeInteger(limits.tenantDayTokens) &&
    safeInteger(limits.globalDayTokens) &&
    safeInteger(limits.tenantMonthCostMicroUsd) &&
    safeInteger(limits.globalMonthCostMicroUsd);
  if (!limitsAreIntegers) return false;

  const tokenDelta = reservation.inputTokens + reservation.outputTokens;
  if (!Number.isSafeInteger(tokenDelta)) return false;
  if (!reservation.paid && reservation.costMicroUsd !== 0) return false;
  if (
    reservation.paid &&
    (reservation.costMicroUsd <= 0 ||
      limits.tenantMonthCostMicroUsd <= 0 ||
      limits.globalMonthCostMicroUsd <= 0)
  ) {
    return false;
  }

  const timestamp = new Date(now).toISOString();
  const minute = utcMinute(now);
  const day = utcDay(now);
  const month = utcMonth(now);
  const tenantScopeKey = TENANT_SCOPE_PREFIX + tenantId;

  const statements = [
    conditionalUpsert(
      repo,
      tenantScopeKey,
      `model-requests-minute:${minute}`,
      1,
      limits.tenantMinute,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      tenantScopeKey,
      `model-requests-day:${day}`,
      1,
      limits.tenantDay,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      tenantScopeKey,
      `model-requests-month:${month}`,
      1,
      limits.tenantMonth,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      GLOBAL_SCOPE_KEY,
      `model-requests-minute:${minute}`,
      1,
      limits.globalMinute,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      GLOBAL_SCOPE_KEY,
      `model-requests-day:${day}`,
      1,
      limits.globalDay,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      tenantScopeKey,
      `model-tokens-day:${day}`,
      tokenDelta,
      limits.tenantDayTokens,
      timestamp,
    ),
    conditionalUpsert(
      repo,
      GLOBAL_SCOPE_KEY,
      `model-tokens-day:${day}`,
      tokenDelta,
      limits.globalDayTokens,
      timestamp,
    ),
  ];
  if (reservation.paid)
    statements.push(
      conditionalUpsert(
        repo,
        tenantScopeKey,
        `model-cost-micro-usd-month:${month}`,
        reservation.costMicroUsd,
        limits.tenantMonthCostMicroUsd,
        timestamp,
      ),
      conditionalUpsert(
        repo,
        GLOBAL_SCOPE_KEY,
        `model-cost-micro-usd-month:${month}`,
        reservation.costMicroUsd,
        limits.globalMonthCostMicroUsd,
        timestamp,
      ),
    );
  const results = await repo.db.batch(statements);

  return (
    results.length === statements.length &&
    results.every((result) => result.success && (result.results?.length ?? 0) === 1)
  );
}
