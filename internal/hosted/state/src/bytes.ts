/**
 * Normalise BLOB reads across engines.
 *
 * Native D1 (and the Miniflare proxy used in tests) returns BLOB columns as
 * number[]; node:sqlite returns Uint8Array/Buffer. Callers must receive a
 * stable byte-array shape regardless of the engine that produced the row.
 */
export function toByteArray(value: unknown): Uint8Array {
  if (value instanceof Uint8Array) return value;
  if (value instanceof ArrayBuffer) return new Uint8Array(value);
  if (Array.isArray(value)) return Uint8Array.from(value as number[]);
  throw new Error("credential read: unsupported blob encoding");
}
