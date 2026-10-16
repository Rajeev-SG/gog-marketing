import { beforeAll, afterAll, expect, it } from "vitest";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";
import { HostedRepository } from "../../../state/src/repository.js";
import { loadMigrationSql, splitMigrationStatements } from "../../../state/src/dev-migrations.js";
import { ChatThreads, conversationForThread, reserveChatBudget } from "./threads.js";
import { chatConversationDigest, chatDraftKey } from "./access.js";

let mf: Miniflare;
let repo: HostedRepository;
beforeAll(async () => {
  mf = new Miniflare(
    convertV4MiniflareOptions({
      workers: [
        {
          name: "threads",
          modules: true,
          script: "export default {};",
          compatibilityDate: "2024-12-01",
          d1Databases: { DB: "threads" },
        },
      ],
    }),
  );
  const db = await mf.getD1Database("DB");
  for (const file of ["0001_initial_schema.sql", "0007_chat_threads.sql"])
    for (const sql of splitMigrationStatements(loadMigrationSql(file))) await db.prepare(sql).run();
  repo = new HostedRepository(db as unknown as D1Database);
});
afterAll(async () => {
  await mf?.dispose();
});

it("preserves the legacy conversation/draft digest and isolates owner/thread metadata", async () => {
  const a = await repo.bootstrapTenant("thread-owner-a"),
    b = await repo.bootstrapTenant("thread-owner-b");
  const ra = new ChatThreads(repo, a.id),
    rb = new ChatThreads(repo, b.id);
  const original = (await ra.list())[0]!;
  expect(original.id).toBe("original");
  expect(original.conversation).toBe(await chatConversationDigest(a.id));
  expect(original.conversation).toBe(await chatDraftKey(a.id));
  const t = (await ra.create("A campaign"))!;
  expect(t.conversation).not.toBe(original.conversation);
  expect(t.conversation).toBe(await conversationForThread(a.id, t.id));
  expect(await rb.get(t.id)).toBeNull();
  expect(await rb.byConversation(t.conversation)).toBeNull();
  expect(await ra.get("not-a-thread")).toBeNull();
  expect((await ra.rename(t.id, "Updated campaign"))?.title).toBe("Updated campaign");
  await ra.touch(t.id, "Must not replace an explicit name");
  expect((await ra.get(t.id))?.title).toBe("Updated campaign");
  await ra.tombstone(t.id);
  expect(await ra.get(t.id)).toBeNull();
  expect(await ra.byConversation(t.conversation)).toBeNull();
  expect((await ra.get(t.id, true))?.purged).toBe(0);
  expect(await ra.pendingDeletions()).toContainEqual({ id: t.id, draftKey: t.conversation });
  expect(await rb.pendingDeletions()).toEqual([]);
  expect((await ra.get(t.id, true))?.title).toBe("Deleted conversation");
  await ra.markPurged(t.id);
  expect((await ra.get(t.id, true))?.purged).toBe(1);
  expect(await ra.pendingDeletions()).toEqual([]);
  await ra.tombstone("original");
  expect(await ra.list()).toHaveLength(0);
  const fresh = (await ra.resolve(null))!;
  expect(fresh.id).not.toBe("original");
  expect(fresh.conversation).not.toBe(original.conversation);
  expect((await ra.get("original", true))?.deletedAt).toBeTruthy();
});
it("atomically caps active threads under concurrent creation", async () => {
  const tenant = await repo.bootstrapTenant("thread-cap");
  const r = new ChatThreads(repo, tenant.id);
  await r.ensureOriginal();
  for (let i = 0; i < 98; i++) expect(await r.create()).not.toBeNull();
  const last = await Promise.all([r.create(), r.create(), r.create()]);
  expect(last.filter(Boolean)).toHaveLength(1);
  expect(await r.list()).toHaveLength(100);
});
it("keeps model budgets account-wide across thread creation and deletion", async () => {
  const tenant = await repo.bootstrapTenant("thread-budget");
  const r = new ChatThreads(repo, tenant.id);
  const t = (await r.create())!;
  const now = Date.now();
  expect(
    await reserveChatBudget(repo, tenant.id, {
      minute: Math.floor(now / 60000),
      minuteCount: 9,
      day: Math.floor(now / 86400000),
      dayCount: 99,
    }),
  ).toBe(true);
  expect(await reserveChatBudget(repo, tenant.id)).toBe(false);
  await r.tombstone(t.id);
  await r.create();
  expect(await reserveChatBudget(repo, tenant.id)).toBe(false);
  const other = await repo.bootstrapTenant("thread-budget-other");
  expect(await reserveChatBudget(repo, other.id)).toBe(true);
});
