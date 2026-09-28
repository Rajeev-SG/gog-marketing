#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ "${BASH_SOURCE[0]}" == *"go-build"* || "${BASH_SOURCE[0]}" == *"/tmp/"* ]]; then
  echo "acceptance-local refuses ephemeral build paths" >&2
  exit 2
fi

bin="$repo_root/bin/gog-acceptance"
if [[ ! -x "$bin" ]]; then
  echo "missing stable acceptance binary; run make build-acceptance" >&2
  exit 2
fi

container="gog-marketing-acceptance-postgres"
port="${GOG_MARKETING_ACCEPTANCE_POSTGRES_PORT:-55433}"
db_url="postgres://gog:gog-test@127.0.0.1:${port}/gog_control_plane?sslmode=disable"
output_root="${GOG_MARKETING_ACCEPTANCE_OUTPUT:-$repo_root/output/acceptance/local}"
profile_root="${GOG_MARKETING_ACCEPTANCE_HOME:-$repo_root/.acceptance/local}"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cleanup
docker run --rm -d --name "$container" \
  -e POSTGRES_USER=gog -e POSTGRES_PASSWORD=gog-test -e POSTGRES_DB=gog_control_plane \
  -p "${port}:5432" postgres:17-alpine >/dev/null

for _ in {1..30}; do
  if docker exec "$container" pg_isready -U gog -d gog_control_plane >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

ACCEPTANCE_JSON=1 "$bin" local \
  --profile-root "$profile_root" \
  --output-root "$output_root" \
  --database-url "$db_url"
