# Bounded review policy — gog-marketing hosted v1

Effective 2026-10-05; owner-authorized replacement of OpenReview for
Rajeev-SG/gog-marketing. Other repositories retain their existing gate.

- OpenReview automatic model review is disabled with `.github/frontier-review.yml`
  (`frontier.enabled: false`). Its skipped success check is not review evidence.
  Keep ClawSweeper issue dispatch; it is a separate service, not OpenReview.
- One review round: two parallel read-only `gpt-6.1-sol-review` roles, high reasoning,
  using Matt Pocock's `/Users/rajeev/.agents/skills/code-review/SKILL.md`.
  Pin the base SHA and exact candidate diff; include the issue and repo standards.
  Standards and Spec reports remain separate, under 400 words each.
- Ten-minute target per reviewer. Controller owns the deadline: request the partial
  report and close a stuck agent; do not wait indefinitely or pretend it passed.
  Verify actual child model/effort from transcript, never from its reply. No fallback.
  Existing Desktop sessions cache role names. If the new role is unavailable, use
  two isolated `codex exec -m gpt-6.1-sol -c model_reasoning_effort="high" -s read-only`
  reviewer processes with a controller-enforced 600-second deadline. These are
  bounded review subagents, not new user-owned chats. A bare model override on
  an old native default role silently used DeepSeek in the live probe; do not use it.
- Block only concrete security defects, incorrect behaviour, missing acceptance,
  or hard documented-standard violations. Fowler smells/preferences are advisory.
  Do not invent a mandatory refactor or repeat tooling-enforced lint findings.
- Collect both reports, one consolidated repair pass, then recheck only those
  findings plus affected tests. Newly discovered concrete regressions still block.
  A second failed repair triggers diagnosis/smaller scope, not another broad review.
- Targeted checks during development; full local CI once per integration candidate;
  GitHub CI on the final SHA. Rerun affected checks after changes; reuse unchanged
  evidence only where valid. Never skip required checks or tests to save time.
- Browser/Google/Codex MCP acceptance, isolation and cost controls remain mandatory.
- Publish a PR comment containing `<!-- bounded-review/v1 -->` and a JSON code block:
  {"schema":"bounded-review/v1","head_sha":"<final SHA>",
   "model":"gpt-6.1-sol","effort":"high","standards":"pass","spec":"pass",
   "blocking_findings":0,"agents":["<standards child ID>","<spec child ID>"]}
  Include reports, transcript verification and any finding-to-test repair map.
  Only publish pass after actual reviews and focused repairs; no synthetic approval.
- Merge guard requires this owner-authored evidence on the exact head and green
  GitHub Actions checks, rather than waiting for OpenReview. Keep all unrelated gates.
- Checkpoint after each meaningful milestone. On stream failure, resume from saved
  state; do not call elapsed wall time productive work. No new scheduled automation.

# Resume #58
#59/#60 landed. #61 is the current frontier; preserve its existing patch and tests.
#62–#69 remain gated by native GitHub blockers. Do not close or rewrite parent #58.
Implementation routing remains the owner's existing GLM Flash/high requirement;
only the review roles change. Coordinator owns integration and real acceptance.
