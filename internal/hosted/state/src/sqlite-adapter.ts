/**
 * Test-only adapter that wraps Node's built-in SQLite (node:sqlite) to match
 * the D1-compatible Database interface. This gives real SQLite behavior for
 * tests — including foreign key enforcement, UNIQUE constraints, and
 * atomic ON CONFLICT updates — rather than mock-string green tests.
 *
 * D1 enables foreign key enforcement by default (equivalent to
 * PRAGMA foreign_keys = on). This adapter enables it explicitly.
 */
import { DatabaseSync } from "node:sqlite";
import { splitMigrationStatements } from "./dev-migrations.js";
import type { D1CompatibleResult, D1CompatibleStatement, Database } from "./types.js";

export interface SQLiteD1Adapter extends Database {
  exec(sql: string): void;
  close(): void;
}

export function createTestDatabase(path = ":memory:"): SQLiteD1Adapter {
  const db = new DatabaseSync(path, { enableForeignKeyConstraints: true });

  const adapter: SQLiteD1Adapter = {
    exec(sql: string): void {
      db.exec(sql);
    },
    close(): void {
      db.close();
    },
    prepare(sql: string) {
      const stmt = db.prepare(sql);
      let boundParams: unknown[] = [];
      const boundSql = sql;

      const bound = {
        // Adapter-only marker consumed by batch() to decide SELECT handling.
        sql: boundSql,
        all<T>(): Promise<{ results?: T[] }> {
          const rows = stmt.all(...(boundParams as never[])) as T[];
          return Promise.resolve({ results: rows });
        },
        first<T>(): Promise<T | null> {
          const row = stmt.get(...(boundParams as never[])) as T | undefined;
          return Promise.resolve(row ?? null);
        },
        run(): Promise<{ success: boolean; meta?: unknown }> {
          try {
            stmt.run(...(boundParams as never[]));
            return Promise.resolve({ success: true });
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            const e = new Error(message) as Error & { cause?: unknown };
            e.cause = err;
            return Promise.reject(e);
          }
        },
      };

      return Object.assign({
        all<T>(): Promise<{ results?: T[] }> {
          return bound.all<T>();
        },
        first<T>(): Promise<T | null> {
          return bound.first<T>();
        },
        run(): Promise<{ success: boolean; meta?: unknown }> {
          return bound.run();
        },
        bind(...values: unknown[]) {
          boundParams = values;
          return bound;
        },
      });
    },
    async batch<T = unknown>(
      statements: D1CompatibleStatement[],
    ): Promise<D1CompatibleResult<T>[]> {
      const results: D1CompatibleResult<T>[] = [];
      // Execute in order like D1 batch. Running the bound statements keeps
      // every binding parameter attached (the previous exec()-based path
      // silently dropped them). SELECT statements return their rows like D1.
      for (const statement of statements) {
        const sql = (statement as { sql?: string }).sql;
        if (typeof sql === "string" && /\bSELECT\b/i.test(sql)) {
          const rows = await statement.all<T>();
          results.push({ success: true, results: rows.results });
        } else {
          const result = await statement.run();
          results.push({ success: result.success });
        }
      }
      return results;
    },
  };

  return adapter;
}

/**
 * Read and execute a SQL migration file against the test database.
 * Splits on `;` at the top level (not inside strings) and strips comments
 * so each statement is valid standalone SQL.
 */
export function applyMigrationSql(db: SQLiteD1Adapter, sql: string): void {
  const statements = splitMigrationStatements(sql);
  for (const stmt of statements) {
    db.exec(stmt);
  }
}
