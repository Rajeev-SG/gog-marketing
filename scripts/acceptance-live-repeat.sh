#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runs="${1:-3}"
bin="$repo_root/bin/gog-acceptance"
for ((i=1; i<=runs; i++)); do
  echo "live run $i/$runs" >&2
  "$bin" live --runs 1
  if [[ "$i" -eq 1 ]]; then
    echo "rebuilding stable gog binary between live runs" >&2
    make -C "$repo_root" build >/dev/null
  fi
done
