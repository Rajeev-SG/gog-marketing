#!/usr/bin/env bash

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TOOLS_DIR="${GOG_SECURITY_TOOLS_DIR:-$ROOT/.tools/security}"
OUT_DIR="${GOG_SECURITY_OUTPUT_DIR:-$ROOT/.tools/security/reports}"
SEMGREP_IMAGE='semgrep/semgrep@sha256:db0579f992f6597840f96b249119f4999d44ffbc9e3f0a8854c367183e474c24'
SCAN_IMAGE="gog-security-scan:$(git -C "$ROOT" rev-parse --short=12 HEAD)"
mkdir -p "$OUT_DIR"
scan_status=0

"$TOOLS_DIR/osv-scanner" --version
"$TOOLS_DIR/trivy" --version
docker image inspect "$SEMGREP_IMAGE" >/dev/null 2>&1 || docker pull "$SEMGREP_IMAGE"

docker run --rm \
  --volume "$ROOT:/src:ro" \
  --volume "$OUT_DIR:/out" \
  --workdir /src \
  --env SEMGREP_SEND_METRICS=off \
  "$SEMGREP_IMAGE" semgrep \
  --config /src/scripts/security/semgrep.yml \
  --no-git-ignore \
  --no-rewrite-rule-ids \
  --json \
  --output /out/semgrep-test.json \
  /src/scripts/security/testdata/semgrep

python3 - "$OUT_DIR/semgrep-test.json" <<'PY'
import json
import sys

results = json.load(open(sys.argv[1]))["results"]
rules = sorted(result["check_id"] for result in results)
expected = [
    "go.dynamic-external-command",
    "typescript.direct-code-execution",
]
if rules != expected:
    raise SystemExit(f"unexpected Semgrep fixture results: {rules}")
print(f"Semgrep fixture: {len(results)} expected findings")
PY

docker run --rm \
  --volume "$ROOT:/src:ro" \
  --volume "$OUT_DIR:/out" \
  --workdir /src \
  --env SEMGREP_SEND_METRICS=off \
  "$SEMGREP_IMAGE" semgrep \
  --error \
  --config /src/scripts/security/semgrep.yml \
  --no-git-ignore \
  --no-rewrite-rule-ids \
  --exclude '.git/**' \
  --exclude '.tools/**' \
  --exclude '**/node_modules/**' \
  --exclude '**/dist/**' \
  --exclude 'scripts/security/testdata/**' \
  --exclude '**/*.test.ts' \
  --exclude '**/*.spec.ts' \
  --exclude 'bin/**' \
  --sarif \
  --output /out/semgrep.sarif \
  /src || scan_status=$?

"$TOOLS_DIR/osv-scanner" scan source \
  --data-source native \
  --offline-vulnerabilities \
  --download-offline-databases \
  --format sarif \
  --output-file "$OUT_DIR/osv-scanner.sarif" \
  --recursive "$ROOT" || scan_status=$?

docker build --tag "$SCAN_IMAGE" "$ROOT"

"$TOOLS_DIR/trivy" config \
  --disable-telemetry \
  --severity HIGH,CRITICAL \
  --format sarif \
  --output "$OUT_DIR/trivy-config.sarif" \
  --exit-code 1 \
  "$ROOT" || scan_status=$?

"$TOOLS_DIR/trivy" image \
  --disable-telemetry \
  --scanners vuln \
  --severity HIGH,CRITICAL \
  --format sarif \
  --output "$OUT_DIR/trivy-image.sarif" \
  --exit-code 1 \
  "$SCAN_IMAGE" || scan_status=$?

if [ "$scan_status" -ne 0 ]; then
  echo "one or more nightly security scans found unresolved findings" >&2
  exit "$scan_status"
fi

