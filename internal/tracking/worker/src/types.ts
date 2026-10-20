export interface Env {
  DB: D1Database;
  TRACKING_KEY: string;
  TRACKING_CURRENT_KEY_VERSION?: string;
  ADMIN_KEY: string;
  [key: `TRACKING_KEY_V${number}`]: string | undefined;
}

export interface PixelPayload {
  r: string; // recipient
  s: string; // subject hash (first 6 chars)
  t: number; // sent timestamp (unix)
}
