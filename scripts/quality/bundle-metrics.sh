#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
build_output="$(pnpm -C "${root}/internal/hosted/app" build)"
printf '%s\n' "${build_output}" >&2

cd "${root}/internal/hosted/app"
BUILD_OUTPUT="${build_output}" node --input-type=module - <<'NODE'
import { readFile, readdir, stat } from "node:fs/promises";
import { gzipSync } from "node:zlib";
import { join } from "node:path";

const files = [];
async function collect(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) await collect(path);
    else files.push(path);
  }
}
await collect("dist/client");
const assets = await Promise.all(
  files.sort().map(async (path) => {
    const bytes = (await stat(path)).size;
    return {
      path,
      bytes,
      gzipBytes: path.endsWith(".html") ? null : gzipSync(await readFile(path)).length,
    };
  }),
);
const match = process.env.BUILD_OUTPUT?.match(/(\d+) modules transformed/);
console.log(JSON.stringify({ modulesTransformed: match ? Number(match[1]) : null, assets }, null, 2));
NODE
