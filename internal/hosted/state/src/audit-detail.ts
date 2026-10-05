/**
 * Audit detail allowlist enforced at the storage boundary.
 *
 * Audit events are the one place where untrusted error context could leak
 * credential material, so detail metadata is restricted to a fixed set of
 * safe keys with strict types. Everything else is rejected before it reaches
 * the database. Error messages are intentionally generic and never echo the
 * supplied values.
 */

const ALLOWED_DETAIL_KEYS = {
  resource: "string",
  resourceType: "string",
  service: "string",
  operation: "string",
  error: "string",
  count: "number",
  page: "number",
  attempt: "number",
  enabled: "boolean",
} as const satisfies Record<string, "string" | "number" | "boolean">;

const MAX_DETAIL_KEYS = 12;
const MAX_DETAIL_STRING_LENGTH = 256;

// Stable internal codes only: never copy a provider exception into "error".
const SAFE_ERROR_CODES = new Set([
  "none",
  "permission_denied",
  "unauthenticated",
  "invalid_request",
  "not_found",
  "rate_limited",
  "quota_exceeded",
  "needs_reconnect",
  "provider_unavailable",
  "internal_error",
]);

// Recognisable credential syntax, not word matching or entropy heuristics.
// Ordinary identifiers such as "token-report-123" remain valid metadata.
const CREDENTIAL_CONTENT =
  /(?:\b(?:access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|code[_-]?verifier|oauth[_-]?code|authorization|cookie|set-cookie|password|secret|token)\b["']?\s*[:=]|\b(?:bearer|basic)\s+\S+|\bya29\.[A-Za-z0-9_-]+|(?:^|\s)1\/\/[A-Za-z0-9_-]+)/i;

export function assertSafeAuditString(value: string): void {
  if (typeof value !== "string" || CREDENTIAL_CONTENT.test(value)) {
    throw new Error("appendAudit: unsafe audit detail rejected");
  }
}

export function sanitizeAuditDetail(detailJson: string): string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(detailJson);
  } catch {
    throw new Error("appendAudit: detail must be valid JSON metadata");
  }

  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("appendAudit: detail must be a flat metadata object");
  }

  const entries = Object.entries(parsed as Record<string, unknown>);
  if (entries.length > MAX_DETAIL_KEYS) {
    throw new Error("appendAudit: detail has too many keys");
  }

  for (const [key, value] of entries) {
    const expectedType = ALLOWED_DETAIL_KEYS[key as keyof typeof ALLOWED_DETAIL_KEYS];
    if (expectedType === undefined) {
      throw new Error("appendAudit: unsafe audit detail rejected");
    }
    if (value === null || typeof value !== expectedType) {
      throw new Error("appendAudit: unsafe audit detail rejected");
    }
    if (expectedType === "string" && (value as string).length > MAX_DETAIL_STRING_LENGTH) {
      throw new Error("appendAudit: unsafe audit detail rejected");
    }
    if (expectedType === "string") {
      assertSafeAuditString(value as string);
      if (key === "error" && !SAFE_ERROR_CODES.has(value as string)) {
        throw new Error("appendAudit: unsafe audit detail rejected");
      }
    }
    if (expectedType === "number" && !Number.isFinite(value as number)) {
      throw new Error("appendAudit: unsafe audit detail rejected");
    }
  }

  return JSON.stringify(Object.fromEntries(entries));
}
