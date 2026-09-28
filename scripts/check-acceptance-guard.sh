#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if rg -n --hidden -S 'go run .*gog|go run \./cmd/gog|go run \./cmd/gog-acceptance' "$repo_root/scripts/acceptance"* "$repo_root/Makefile" "$repo_root/internal/acceptance" "$repo_root/cmd/gog-acceptance" >/tmp/acceptance-go-run.txt; then
  echo "acceptance harness must not use go run" >&2
  cat /tmp/acceptance-go-run.txt >&2
  exit 1
fi
echo "acceptance guard: PASS"
