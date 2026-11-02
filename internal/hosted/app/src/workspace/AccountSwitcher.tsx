import { Select } from "../ui/index.js";
import { useWorkspace } from "./state.js";
import { navigate, useLocation } from "./router.js";
import { accountName, enabledCount, safeStatus } from "./model.js";

/** Which Google account a Data access or AI tools page is editing. Hidden for one account. */
export function AccountSwitcher() {
  const ws = useWorkspace();
  const url = useLocation();
  if (ws.connections.length < 2 || !ws.selected) return null;
  return (
    <Select
      label="Google account"
      className="account-switcher"
      value={ws.selected.id}
      options={ws.connections.map((connection) => {
        const [tone, text] = safeStatus(connection);
        return {
          id: connection.id,
          label: accountName(connection),
          description:
            `${enabledCount(connection)} assets enabled` + (tone !== "ok" ? ` · ${text}` : ""),
        };
      })}
      onChange={async (id) => {
        if (!(await ws.selectAccount(id))) return;
        const next = new URL(url);
        next.searchParams.set("account", id);
        next.searchParams.delete("service");
        navigate(next.pathname + next.search, true);
      }}
    />
  );
}
