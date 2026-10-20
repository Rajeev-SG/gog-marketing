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

"$TOOLS_DIR/zizmor" \
  --offline \
  --config "$ROOT/scripts/security/zizmor.yml" \
  --format sarif \
  "$ROOT/.github/workflows" \
  > "$OUT_DIR/zizmor.sarif"

