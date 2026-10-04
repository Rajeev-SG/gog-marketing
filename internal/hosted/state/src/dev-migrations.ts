/**
 * Dev/test-only migration loader.
 *
 * This module deliberately uses Node built-ins and MUST NOT be imported from
 * the production entrypoint (`src/index.ts`). Production bundles inline or
 * apply migrations through Wrangler's migration tooling, not this helper.
 */
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const migrationsDir = join(here, "..", "migrations");

export function loadMigrationSql(filename: string): string {
  return readFileSync(join(migrationsDir, filename), "utf-8");
}

/**
 * Split SQL migration text into standalone statements on top-level
 * semicolons, ignoring semicolons inside strings and comments.
 */
export function splitMigrationStatements(sql: string): string[] {
  const statements: string[] = [];
  let current = "";
  let inString = false;
  let inLineComment = false;
  let inBlockComment = false;

  let i = 0;
  while (i < sql.length) {
    const ch = sql[i];
    const next = i + 1 < sql.length ? sql[i + 1] : "\0";

    if (inLineComment) {
      if (ch === "\n") inLineComment = false;
      i++;
      continue;
    }
    if (inBlockComment) {
      if (ch === "*" && next === "/") {
        inBlockComment = false;
        i += 2;
      } else {
        i++;
      }
      continue;
    }
    if (inString) {
      current += ch;
      if (ch === "'") {
        if (next === "'") {
          current += next;
          i += 2;
        } else {
          inString = false;
          i++;
        }
      } else {
        i++;
      }
      continue;
    }
    if (ch === "-" && next === "-") {
      inLineComment = true;
      i += 2;
      continue;
    }
    if (ch === "/" && next === "*") {
      inBlockComment = true;
      i += 2;
      continue;
    }
    if (ch === "'") {
      inString = true;
      current += ch;
      i++;
      continue;
    }
    if (ch === ";") {
      if (current.trim()) statements.push(current.trim());
      current = "";
      i++;
      continue;
    }
    current += ch;
    i++;
  }
  if (current.trim()) statements.push(current.trim());
  return statements;
}
