# Hosted MCP quota and observability

This page is for operators. The queries below return aggregate usage and
stable error categories only. They do not return OAuth material, Google
credentials, raw provider errors, customer email addresses, bearer tokens, or
raw IP addresses. IP rate counters store a SHA-256 digest.

## Enforcement settings

| Worker variable | Default | Window |
| --- | ---: | --- |
| `GOG_MCP_TOOL_CALL_DAILY_LIMIT` | `100` | UTC calendar day |
| `GOG_MCP_REQUESTS_PER_MINUTE_LIMIT` | `60` | UTC fixed minute, separately for tenant and hashed source IP |

Both variables must be positive integers. Invalid values fail `/mcp` closed
with a structured `-32603 invalid_server_configuration` response.

`tools/call` atomically consumes one daily quota unit before policy checks or
runner work. Exhaustion returns HTTP 429 and JSON-RPC `-32003` with
`retryAfterSeconds`. If quota or rate counters fail, the request is denied with
HTTP 503 and JSON-RPC `-32603`; no uncounted tool call is executed.

Request rate exhaustion returns HTTP 429 and JSON-RPC `-32029`, including the
offending scope (`tenant` or `ip`) and `retryAfterSeconds`.

## Audit signals

| Action | Result | Meaning |
| --- | --- | --- |
| `mcp.tools.call` | `allow` | Executed tool call |
| `mcp.tools.call` | `deny` | Policy rejection or `quota_exceeded` |
| `mcp.tools.call` | `error` | Quota-counter, credential, runner, or execution failure |
| `mcp.request.rate_limited` | `deny` | Authenticated tenant request limit rejection |
| `mcp.request` | `error` | Request rate-counter failure |

Every `mcp.tools.call` row records integer `latency_ms`, measured from entry to
`tools/call` up to immediately before the audit write. It excludes the audit
write itself. Successful rows include credential retrieval and runner
round-trip time.

Authentication and protocol failures that occur before tenant resolution are
aggregated in `hosted_mcp_signal_counters`. Rows contain only a fixed signal
label, UTC day, count, and update timestamp. They contain no bearer value,
request body, tenant identifier, or IP address. `mcp_request_rate_limited` is
also incremented for tenant-independent hashed-IP limit rejections.

## Operator queries

Run these read-only queries through the operator's authenticated Wrangler D1
session from `internal/hosted/state`:

```sh
npx wrangler d1 execute gog-marketing \
  --remote \
  --config wrangler.migrations.toml \
  --json \
  --command '<SQL>'
```

### Per-day invocation and outcome counts

```sql
SELECT substr(created_at, 1, 10) AS day,
       count(*) AS attempts,
       sum(CASE WHEN result = 'allow' THEN 1 ELSE 0 END) AS allowed,
       sum(CASE WHEN result = 'deny' THEN 1 ELSE 0 END) AS denied,
       sum(CASE WHEN result = 'error' THEN 1 ELSE 0 END) AS errors
FROM hosted_audit_events
WHERE action = 'mcp.tools.call'
GROUP BY day
ORDER BY day DESC;
```

The admitted-call counter is also available directly:

```sql
SELECT period AS day, sum(value) AS admitted_tool_calls
FROM hosted_quota_counters
WHERE counter = 'mcp_tool_calls'
GROUP BY period
ORDER BY period DESC;
```

### Allow, deny, and error counts by signal

```sql
SELECT substr(created_at, 1, 10) AS day, action, result, count(*) AS events
FROM hosted_audit_events
WHERE action IN ('mcp.tools.call', 'mcp.request.rate_limited', 'mcp.request')
GROUP BY day, action, result
ORDER BY day DESC, action, result;
```

### Authentication and MCP protocol signals

```sql
SELECT period AS day, signal, sum(value) AS events
FROM hosted_mcp_signal_counters
WHERE signal IN (
  'auth_missing_bearer',
  'auth_invalid_bearer',
  'auth_error',
  'auth_tenant_denied',
  'mcp_parse_error',
  'mcp_invalid_request',
  'mcp_method_not_found',
  'mcp_internal_error',
  'mcp_request_rate_limited'
)
GROUP BY day, signal
ORDER BY day DESC, signal;
```

### Quota and abuse rejections

```sql
SELECT substr(created_at, 1, 10) AS day,
       sum(CASE WHEN detail_json LIKE '%"error":"quota_exceeded"%' THEN 1 ELSE 0 END)
         AS quota_rejections,
       sum(CASE WHEN action = 'mcp.request.rate_limited'
                 AND detail_json LIKE '%"operation":"tenant"%' THEN 1 ELSE 0 END)
         AS tenant_rate_rejections,
       sum(CASE WHEN action = 'mcp.request.rate_limited'
                 AND detail_json LIKE '%"operation":"ip"%' THEN 1 ELSE 0 END)
         AS ip_rate_rejections
FROM hosted_audit_events
WHERE detail_json LIKE '%"error":"quota_exceeded"%'
   OR action = 'mcp.request.rate_limited'
GROUP BY day
ORDER BY day DESC;
```

### Stable error categories, including Cloud Run-facing failures

```sql
SELECT substr(created_at, 1, 10) AS day,
       detail_json LIKE '%"error":"provider_unavailable"%' AS provider_unavailable,
       detail_json LIKE '%"error":"needs_reconnect"%' AS needs_reconnect,
       detail_json LIKE '%"error":"internal_error"%' AS internal_error,
       count(*) AS errors
FROM hosted_audit_events
WHERE action = 'mcp.tools.call' AND result = 'error'
GROUP BY day, provider_unavailable, needs_reconnect, internal_error
ORDER BY day DESC;
```

Raw Cloud Run response bodies and exceptions are intentionally not stored.
Provider-side status and cold-start detail remain in the operator's Cloud Run
logs; `latency_ms` is the end-to-end tool signal available in D1.

### Successful tool latency p50/p95

Replace the two timestamps with the UTC range to inspect:

```sql
WITH samples AS (
  SELECT latency_ms
  FROM hosted_audit_events
  WHERE action = 'mcp.tools.call'
    AND result = 'allow'
    AND latency_ms IS NOT NULL
    AND created_at >= '2026-10-08T00:00:00.000Z'
    AND created_at <  '2026-10-09T00:00:00.000Z'
),
ordered AS (
  SELECT latency_ms,
         row_number() OVER (ORDER BY latency_ms) AS rank,
         count(*) OVER () AS sample_count
  FROM samples
)
SELECT count(*) AS samples,
       max(CASE WHEN rank = (sample_count * 50 + 99) / 100
                THEN latency_ms END) AS p50_ms,
       max(CASE WHEN rank = (sample_count * 95 + 99) / 100
                THEN latency_ms END) AS p95_ms
FROM ordered;
```

### Request pressure

This query contains no raw IP values:

```sql
SELECT substr(period, 1, 10) AS day,
       scope_kind,
       count(*) AS active_windows,
       sum(value) AS requests
FROM hosted_rate_limit_counters
GROUP BY day, scope_kind
ORDER BY day DESC, scope_kind;
```
