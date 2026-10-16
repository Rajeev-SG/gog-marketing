/** Owner-scoped thread index. Transcripts and approvals stay in native Chat DOs. */
import { HostedRepository } from "../../../state/src/repository.js";
import { chatConversationDigest } from "./access.js";

export interface ChatThread {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}
export interface ThreadRecord extends ChatThread {
  conversation: string;
  deletedAt: string | null;
  purged: number;
}
const COLUMNS =
  "id,title,conversation,created_at AS createdAt,updated_at AS updatedAt,deleted_at AS deletedAt,purged";
export const validThreadId = (id: string) =>
  id === "original" ||
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id);
export const publicThread = ({ id, title, createdAt, updatedAt }: ThreadRecord): ChatThread => ({
  id,
  title,
  createdAt,
  updatedAt,
});
export function threadTitle(raw: unknown): string {
  if (typeof raw !== "string" || !raw.trim() || raw.trim().length > 100)
    throw new Error("Invalid conversation name.");
  return raw.trim();
}
export async function conversationForThread(tenantId: string, id: string): Promise<string> {
  return chatConversationDigest(id === "original" ? tenantId : tenantId + ":thread:" + id);
}
export class ChatThreads {
  constructor(
    private repo: HostedRepository,
    readonly tenantId: string,
  ) {}
  async ensureOriginal() {
    const now = new Date().toISOString();
    await this.repo.db
      .prepare(
        "INSERT OR IGNORE INTO hosted_chat_threads(tenant_id,id,conversation,title,created_at,updated_at) VALUES(?1,'original',?2,'Original conversation',?3,?3)",
      )
      .bind(this.tenantId, await conversationForThread(this.tenantId, "original"), now)
      .run();
  }
  async list(): Promise<ThreadRecord[]> {
    await this.ensureOriginal();
    const rows = await this.repo.db
      .prepare(
        `SELECT ${COLUMNS} FROM hosted_chat_threads WHERE tenant_id=?1 AND deleted_at IS NULL ORDER BY updated_at DESC,id LIMIT 100`,
      )
      .bind(this.tenantId)
      .all<ThreadRecord>();
    return rows.results ?? [];
  }
  async pendingDeletions(): Promise<{ id: string; draftKey: string }[]> {
    const rows = await this.repo.db
      .prepare(
        "SELECT id,conversation AS draftKey FROM hosted_chat_threads WHERE tenant_id=?1 AND deleted_at IS NOT NULL AND purged=0 ORDER BY deleted_at,id LIMIT 100",
      )
      .bind(this.tenantId)
      .all<{ id: string; draftKey: string }>();
    return rows.results ?? [];
  }
  async get(id: string, includeDeleted = false): Promise<ThreadRecord | null> {
    if (!validThreadId(id)) return null;
    return this.repo.db
      .prepare(
        `SELECT ${COLUMNS} FROM hosted_chat_threads WHERE tenant_id=?1 AND id=?2${includeDeleted ? "" : " AND deleted_at IS NULL"}`,
      )
      .bind(this.tenantId, id)
      .first<ThreadRecord>();
  }
  async byConversation(conversation: string, includeDeleted = false): Promise<ThreadRecord | null> {
    return this.repo.db
      .prepare(
        `SELECT ${COLUMNS} FROM hosted_chat_threads WHERE tenant_id=?1 AND conversation=?2${includeDeleted ? "" : " AND deleted_at IS NULL"}`,
      )
      .bind(this.tenantId, conversation)
      .first<ThreadRecord>();
  }
  async create(title = "New chat"): Promise<ThreadRecord | null> {
    await this.ensureOriginal();
    title = threadTitle(title);
    const id = crypto.randomUUID();
    const now = new Date().toISOString();
    // One atomic statement prevents concurrent creates exceeding the owner cap.
    return this.repo.db
      .prepare(
        `INSERT INTO hosted_chat_threads(tenant_id,id,conversation,title,created_at,updated_at) SELECT ?1,?2,?3,?4,?5,?5 WHERE (SELECT COUNT(*) FROM hosted_chat_threads WHERE tenant_id=?1 AND deleted_at IS NULL)<100 RETURNING ${COLUMNS}`,
      )
      .bind(this.tenantId, id, await conversationForThread(this.tenantId, id), title, now)
      .first<ThreadRecord>();
  }
  async resolve(id: string | null): Promise<ThreadRecord | null> {
    await this.ensureOriginal();
    if (id !== null) return this.get(id);
    return (await this.list())[0] ?? this.create();
  }
  async rename(id: string, title: string): Promise<ThreadRecord | null> {
    return this.repo.db
      .prepare(
        `UPDATE hosted_chat_threads SET title=?3,updated_at=?4 WHERE tenant_id=?1 AND id=?2 AND deleted_at IS NULL RETURNING ${COLUMNS}`,
      )
      .bind(this.tenantId, id, threadTitle(title), new Date().toISOString())
      .first<ThreadRecord>();
  }
  async touch(id: string, firstText: string) {
    const title = firstText.trim().split(/\r?\n/)[0]?.slice(0, 100) || "New chat";
    await this.repo.db
      .prepare(
        "UPDATE hosted_chat_threads SET title=CASE WHEN title='New chat' THEN ?3 ELSE title END,updated_at=?4 WHERE tenant_id=?1 AND id=?2 AND deleted_at IS NULL",
      )
      .bind(this.tenantId, id, title, new Date().toISOString())
      .run();
  }
  async tombstone(id: string): Promise<ThreadRecord | null> {
    await this.repo.db
      .prepare(
        "UPDATE hosted_chat_threads SET deleted_at=COALESCE(deleted_at,?3),title='Deleted conversation'  WHERE tenant_id=?1 AND id=?2",
      )
      .bind(this.tenantId, id, new Date().toISOString())
      .run();
    return this.get(id, true);
  }
  async markPurged(id: string) {
    await this.repo.db
      .prepare(
        "UPDATE hosted_chat_threads SET purged=1 WHERE tenant_id=?1 AND id=?2 AND deleted_at IS NOT NULL",
      )
      .bind(this.tenantId, id)
      .run();
  }
}
export interface LegacyBudget {
  minute: number;
  minuteCount: number;
  day: number;
  dayCount: number;
}
export async function seedLegacyChatBudget(
  repo: HostedRepository,
  tenantId: string,
  legacy?: LegacyBudget,
): Promise<void> {
  const now = Date.now();
  const counters = [
    {
      period: "chat-minute:" + Math.floor(now / 60000),
      count: legacy?.minute === Math.floor(now / 60000) ? legacy.minuteCount : 0,
    },
    {
      period: "chat-day:" + Math.floor(now / 86400000),
      count: legacy?.day === Math.floor(now / 86400000) ? legacy.dayCount : 0,
    },
  ];
  for (const counter of counters)
    if (counter.count > 0)
      await repo.db
        .prepare(
          "INSERT INTO hosted_quota_counters(tenant_id,period,counter,value,updated_at) VALUES(?1,?2,'chat_model_starts',?3,?4) ON CONFLICT(tenant_id,period,counter) DO UPDATE SET value=MAX(value,excluded.value)",
        )
        .bind(tenantId, counter.period, counter.count, new Date().toISOString())
        .run();
}
export async function reserveChatBudget(
  repo: HostedRepository,
  tenantId: string,
  legacy?: LegacyBudget,
): Promise<boolean> {
  await seedLegacyChatBudget(repo, tenantId, legacy);
  const now = Date.now();
  if (
    (await repo.incrementQuotaWithinLimit(
      tenantId,
      "chat-minute:" + Math.floor(now / 60000),
      "chat_model_starts",
      10,
    )) === null
  )
    return false;
  return (
    (await repo.incrementQuotaWithinLimit(
      tenantId,
      "chat-day:" + Math.floor(now / 86400000),
      "chat_model_starts",
      100,
    )) !== null
  );
}
