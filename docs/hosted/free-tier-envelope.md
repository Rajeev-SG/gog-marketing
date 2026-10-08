# Hosted v1 free-tier envelope

Provider limits verified from current official documentation on 2026-10-08.
Provider pricing and free tiers can change; re-check the linked pages before a
capacity or billing decision.

## Current envelope

| Service | Free allowance relevant to hosted v1 |
| --- | --- |
| Cloudflare Workers Free | 100,000 requests/day; 10 ms CPU per HTTP request; 128 MB per isolate; 50 subrequests/request |
| Cloudflare D1 Free | 5 million rows read/day; 100,000 rows written/day; 5 GB total stored data |
| Cloudflare KV Free | 100,000 key reads/day; 1,000 key writes/day; 1,000 key deletes/day; 1,000 list requests/day; 1 GB stored data |
| Cloud Run, request-based billing | 2 million requests/month; 180,000 vCPU-seconds/month; 360,000 GiB-seconds/month; 1 GiB North America data transfer/month |

Cloudflare Free limits reset daily at 00:00 UTC. Since 2026-09-01, D1 enforces
its free row-read and row-write ceilings by failing further queries until the
next UTC day. KV is not used by the current hosted state path.

Cloud Run's current pricing page also publishes a separate instance-based
billing envelope: 240,000 vCPU-seconds and 450,000 GiB-seconds per month, with
no request allowance. Hosted v1 must use request-based billing to preserve the
invocation-based envelope and scale-to-zero cost behaviour described below.

## First paid bottleneck

The first automatic paid overage is expected at the private Cloud Run runner,
through request count and CPU time. At the beta soft ceiling of 500 users and
100 tool calls/user/day, one runner request per tool call is approximately
1.5 million requests/month. That remains under the 2 million request allowance
only before discovery, retries, or other runner traffic.

At 1.5 million runner requests/month, the request-based free CPU allowance is
about 120 ms of vCPU time per request on average
(`180,000 / 1,500,000 = 0.12` vCPU-seconds). The memory allowance is about
0.24 GiB-seconds per request. Runner CPU or request growth beyond those
budgets is therefore the expected first charge.

There is also earlier hard-limit pressure that fails rather than bills:

- Workers allows 100,000 requests/day. Fifty thousand tool calls leave only
  50,000 requests/day for authentication, discovery, `tools/list`, UI/API use,
  and retries.
- D1 allows 100,000 row writes/day. With a source IP available, each accepted
  tool call performs at least four logical writes: two fixed-window rate
  counters, one quota counter, and one audit row. D1 counts writes to indexes
  as additional row writes, so the practical ceiling is lower. The D1 write
  limit can therefore be reached before Cloud Run's free compute allowance.

Operators should watch the aggregate queries in
[`docs/hosted/observability.md`](observability.md) before admitting the full
500-user envelope.

## No fixed monthly spend from scaling

The live Cloud Run runner uses `maxScale=20` as a safety ceiling. A maximum
instance count does not reserve capacity and does not by itself create fixed
monthly spend. The following invariants keep hosted v1 usage-based:

- Cloud Run request-based billing remains enabled.
- Minimum instances remains `0` at both service and revision scope.
- No committed-use discount or always-on instance is configured.

Minimum instances must stay `0`. Under request-based billing, Google documents
that zero minimum instances means idle time is not billed. Under
instance-based billing, Cloud Run bills the full instance lifecycle even with
minimum instances set to `0`, so that billing mode must not be substituted
without a separate cost review.

## Sources

- [Cloudflare Workers limits](https://developers.cloudflare.com/workers/platform/limits/)
- [Cloudflare Workers, D1, and KV pricing](https://developers.cloudflare.com/workers/platform/pricing/)
- [Cloudflare D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/)
- [Cloud Run pricing](https://cloud.google.com/run/pricing)
- [Cloud Run minimum instances](https://cloud.google.com/run/docs/configuring/min-instances)
- [Cloud Run maximum instances](https://cloud.google.com/run/docs/configuring/max-instances)

