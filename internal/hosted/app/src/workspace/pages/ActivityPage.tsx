import { Button, Icon, SegmentedControl } from "../../ui/index.js";
import { useWorkspace } from "../state.js";
import { navigate, useLocation } from "../router.js";
import { accountName, type ActivityItem } from "../model.js";

const FILTERS = [
  { id: "all", label: "All activity" },
  { id: "problems", label: "Problems" },
  { id: "tools", label: "AI tool use" },
  { id: "settings", label: "Setting changes" },
] as const;
type Filter = (typeof FILTERS)[number]["id"];

const TONE_LABEL: Record<ActivityItem["tone"], string> = {
  ok: "Done",
  blocked: "Blocked",
  failed: "Failed",
  info: "Change",
};
const TONE_ICON: Record<ActivityItem["tone"], string> = {
  ok: "check",
  blocked: "alert",
  failed: "alert",
  info: "circle",
};

function matches(event: ActivityItem, filter: Filter) {
  if (filter === "problems") return event.tone === "blocked" || event.tone === "failed";
  if (filter === "tools") return event.kind === "tool";
  if (filter === "settings") return event.kind === "setting";
  return true;
}

const day = (iso: string) =>
  new Date(iso).toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" });
const time = (iso: string) =>
  new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });

/** Settings › Activity: what AI tools and you did, newest first (N01). */
export function ActivityPage() {
  const ws = useWorkspace();
  const url = useLocation();
  const requested = url.searchParams.get("show");
  const filter: Filter = FILTERS.some((item) => item.id === requested)
    ? (requested as Filter)
    : "all";
  const events = (ws.activity ?? []).filter((event) => matches(event, filter));
  const days = new Map<string, ActivityItem[]>();
  for (const event of events) {
    const key = day(event.at);
    if (!days.has(key)) days.set(key, []);
    days.get(key)!.push(event);
  }
  const account = (id: string) => {
    const connection = ws.connections.find((item) => item.id === id);
    return connection ? accountName(connection) : "";
  };

  return (
    <>
      <header className="page-header">
        <div>
          <h1>Activity</h1>
          <p>What AI tools did with your Google data, and the changes you made. Newest first.</p>
        </div>
      </header>
      <section className="panel" aria-labelledby="activity-heading">
        <div className="panel-head activity-head">
          <h2 id="activity-heading" className="sr-only">
            Activity log
          </h2>
          <SegmentedControl
            label="Show"
            options={FILTERS}
            value={filter}
            onChange={(next) =>
              navigate("/settings/activity" + (next === "all" ? "" : `?show=${next}`), true)
            }
          />
          <Button size="small" onPress={() => void ws.reloadActivity()}>
            Refresh
          </Button>
        </div>
        {ws.activityState === "loading" && !ws.activity && (
          <p className="activity-empty" role="status">
            Loading activity…
          </p>
        )}
        {ws.activityState === "error" && (
          <div className="activity-empty" role="alert">
            <p>We couldn’t load activity. Nothing else is affected.</p>
            <Button onPress={() => void ws.reloadActivity()}>Try again</Button>
          </div>
        )}
        {ws.activity && events.length === 0 && (
          <p className="activity-empty">
            {filter === "problems"
              ? "No problems recorded. Blocked or failed AI requests would appear here."
              : "No activity to show yet."}
          </p>
        )}
        {[...days.entries()].map(([label, items]) => (
          <section key={label} className="activity-day" aria-label={label}>
            <h3>{label}</h3>
            <ol className="activity-list">
              {items.map((event) => (
                <li key={event.id} className={"activity-item " + event.tone}>
                  <span className="activity-icon">
                    <Icon name={TONE_ICON[event.tone]} />
                  </span>
                  <div className="activity-copy">
                    <strong>{event.title}</strong>
                    {(event.detail || account(event.connectionId)) && (
                      <p>
                        {[event.detail, account(event.connectionId)].filter(Boolean).join(" · ")}
                      </p>
                    )}
                  </div>
                  <span className="activity-meta">
                    <span className="sr-only">{TONE_LABEL[event.tone]}, </span>
                    <time dateTime={event.at}>{time(event.at)}</time>
                  </span>
                </li>
              ))}
            </ol>
          </section>
        ))}
        {ws.activity && ws.activity.length >= 200 && (
          <p className="activity-empty">Showing the latest 200 events.</p>
        )}
      </section>
    </>
  );
}
