import { generateKeyPairSync } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";

const here = dirname(fileURLToPath(import.meta.url));
const appRoot = join(here, "..");
const migrationRoot = join(appRoot, "..", "state", "migrations");
const port = Number(process.env.PLAYWRIGHT_PORT ?? 8788);
const origin = "https://test.example.com";
const clerkIssuer = "https://oriented-wombat-2813.clerk.accounts.dev";
const clerkPublishableKey = "pk_test_b3JpZW50ZWQtd29tYmF0LTI4MTMuY2xlcmsuYWNjb3VudHMuZGV2JA";

const bundle = await build({
  entryPoints: [join(appRoot, "src", "index.ts")],
  bundle: true,
  format: "esm",
  platform: "browser",
  target: "es2022",
  write: false,
});
const workerModule = await import(
  `data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}`
);
const worker = workerModule.default;

const { publicKey } = generateKeyPairSync("rsa", {
  modulusLength: 2048,
  publicKeyEncoding: { type: "spki", format: "pem" },
  privateKeyEncoding: { type: "pkcs8", format: "pem" },
});

function migrationStatements(sql) {
  return sql
    .replace(/--.*$/gm, "")
    .split(";")
    .map((statement) => statement.trim())
    .filter(Boolean);
}

const miniflare = new Miniflare(
  convertV4MiniflareOptions({
    workers: [
      {
        name: "hosted-app-e2e",
        compatibilityDate: "2024-12-01",
        d1Databases: { DB: "hosted-app-e2e" },
        script: bundle.outputFiles[0].text,
        modules: true,
        bindings: {
          CLERK_SECRET_KEY: "sk_test_local_e2e_only",
          CLERK_PUBLISHABLE_KEY: clerkPublishableKey,
          CLERK_ISSUER: clerkIssuer,
          CLERK_AUTHORIZED_PARTIES: origin,
          CLERK_JWT_KEY: publicKey,
          GOG_HOSTED_CANONICAL_ORIGIN: origin,
          GOG_GOOGLE_OAUTH_CLIENT_ID: "local-e2e-client-id",
          GOG_GOOGLE_OAUTH_CLIENT_SECRET: "local-e2e-client-secret",
          GOG_GOOGLE_OAUTH_REDIRECT_URI: `${origin}/oauth/google/callback`,
          GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: Buffer.alloc(32, 7).toString("base64"),
        },
      },
    ],
  }),
);

const db = await miniflare.getD1Database("DB");
for (const migration of [
  "0001_initial_schema.sql",
  "0002_oauth_states_discovery.sql",
  "0003_rate_limit_counters.sql",
  "0004_mcp_signal_counters.sql",
]) {
  const sql = await readFile(join(migrationRoot, migration), "utf8");
  for (const statement of migrationStatements(sql)) {
    const result = await db.batch([db.prepare(statement)]);
    if (!result[0]?.success) {
      throw new Error(`migration statement failed: ${migration}`);
    }
  }
}

const env = {
  DB: db,
  CLERK_SECRET_KEY: "sk_test_local_e2e_only",
  CLERK_PUBLISHABLE_KEY: clerkPublishableKey,
  CLERK_ISSUER: clerkIssuer,
  CLERK_AUTHORIZED_PARTIES: origin,
  CLERK_JWT_KEY: publicKey,
  GOG_HOSTED_CANONICAL_ORIGIN: origin,
  GOG_GOOGLE_OAUTH_CLIENT_ID: "local-e2e-client-id",
  GOG_GOOGLE_OAUTH_CLIENT_SECRET: "local-e2e-client-secret",
  GOG_GOOGLE_OAUTH_REDIRECT_URI: `${origin}/oauth/google/callback`,
  GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY: Buffer.alloc(32, 7).toString("base64"),
};

const server = createServer(async (request, response) => {
  const headers = new Headers();
  for (const [name, value] of Object.entries(request.headers)) {
    if (
      value === undefined ||
      name === "host" ||
      name === "connection" ||
      name === "accept" ||
      name === "origin" ||
      name === "referer" ||
      name.startsWith("sec-fetch-")
    ) {
      continue;
    }
    headers.set(name, Array.isArray(value) ? value.join(", ") : value);
  }

  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  const body = chunks.length ? new Uint8Array(Buffer.concat(chunks)) : undefined;
  const upstream = await worker.fetch(
    new Request(`${origin}${request.url ?? "/"}`, {
      method: request.method ?? "GET",
      headers,
      body,
    }),
    env,
  );

  response.statusCode = upstream.status;
  for (const [name, value] of upstream.headers) response.setHeader(name, value);
  response.end(Buffer.from(await upstream.arrayBuffer()));
});

server.listen(port, "127.0.0.1", () => {
  console.log(`hosted-app e2e fixture listening on http://127.0.0.1:${port}`);
});

async function shutdown() {
  server.close();
  await miniflare.dispose();
  process.exit(0);
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
