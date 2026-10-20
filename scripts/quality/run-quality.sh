#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
knip_version="6.42.1"
jscpd_version="5.4.1"

run_knip() {
  local package="$1"
  echo "Knip: ${package}"
  (cd "${root}/${package}" && pnpm dlx "knip@${knip_version}" --config knip.json)
}

run_jscpd() {
  local package="$1"
  local sources="$2"
  local ignore="$3"
  echo "jscpd: ${package}"
  (cd "${root}/${package}" && pnpm dlx "jscpd@${jscpd_version}" "${sources}" \
    --format "typescript,tsx" --min-tokens 100 --ignore "${ignore}" \
    --threshold 1 --reporters silent --silent)
}

run_knip internal/hosted/app
run_knip internal/hosted/state
run_knip internal/tracking/worker

run_jscpd internal/hosted/app src "**/*.test.ts,**/product-css.generated.ts,**/tool-catalog.generated.json"
run_jscpd internal/hosted/state src "**/*.test.ts"
run_jscpd internal/tracking/worker src "**/*.test.ts"

echo "jscpd: Go production sources"
pnpm -C "${root}" dlx "jscpd@${jscpd_version}" cmd internal \
  --format go --min-tokens 100 \
  --ignore "**/*_test.go,**/node_modules/**,**/dist/**,**/vendor/**" \
  --threshold 1 --reporters silent --silent
