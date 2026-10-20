#!/usr/bin/env bash

set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TOOLS_DIR="${GOG_SECURITY_TOOLS_DIR:-$ROOT/.tools/security}"
OUT_DIR="${GOG_SECURITY_OUTPUT_DIR:-$ROOT/.tools/security/reports}"
cd "$ROOT"
mkdir -p "$OUT_DIR"

"$TOOLS_DIR/gitleaks" version
"$TOOLS_DIR/zizmor" --version

"$TOOLS_DIR/gitleaks" dir . \
  --no-banner \
  --no-color \
  --redact \
  --gitleaks-ignore-path scripts/security \
  --report-format sarif \
  --report-path "$OUT_DIR/gitleaks.sarif"

# SARIF deliberately exits zero even for high findings. Both the proof and real
# gate share plain output so changing the invocation cannot silently weaken CI.
scan_workflows() {
  "$TOOLS_DIR/zizmor" \
    --offline \
    --config "$ROOT/scripts/security/zizmor.yml" \
    --format plain \
    "$1"
}

if scan_workflows "$ROOT/scripts/security/testdata/zizmor/unsafe.yml" > "$OUT_DIR/zizmor-gate-proof.log" 2>&1; then
  echo "workflow gate accepted the known-unsafe fixture" >&2
  exit 1
else
  proof_status=$?
  if [ "$proof_status" -ne 14 ]; then
    cat "$OUT_DIR/zizmor-gate-proof.log" >&2
    echo "expected high-finding exit14, got $proof_status" >&2
    exit 1
  fi
fi

scan_workflows "$ROOT/.github/workflows" 2>&1 | tee "$OUT_DIR/zizmor.log"
