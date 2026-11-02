/**
 * Information architecture (D05, refreshed 2 Nov 2026).
 *
 * The sidebar has three destinations: Home, Chat and Settings. Everything the
 * user configures lives under Settings, split into sub-pages that each show
 * whether they need attention.
 */
export const SETTINGS_SECTIONS = [
  { id: "overview", label: "Overview", href: "/settings" },
  { id: "accounts", label: "Google accounts", href: "/settings/accounts" },
  { id: "data", label: "Data access", href: "/settings/data" },
  { id: "tools", label: "AI tools", href: "/settings/tools" },
  { id: "activity", label: "Activity", href: "/settings/activity" },
  { id: "connect-ai", label: "Connect AI apps", href: "/settings/connect-ai" },
] as const;
export type SettingsSectionId = (typeof SETTINGS_SECTIONS)[number]["id"];

/** Signed-in pages served by the React workspace app. */
export const WORKSPACE_PATHS = ["/", ...SETTINGS_SECTIONS.map((section) => section.href)] as const;

/** Pages from the previous IA. The server redirects them permanently, keeping the query. */
export const LEGACY_WORKSPACE_PATHS: Readonly<Record<string, string>> = {
  "/data": "/settings/data",
  "/access": "/settings/accounts",
  "/connect-ai": "/settings/connect-ai",
};

/** Old single-page anchors (/#google-data, …) still land on the right page. */
export const LEGACY_WORKSPACE_ANCHORS: Readonly<Record<string, string>> = {
  home: "/",
  "google-data": "/settings/data",
  "asset-workspace": "/settings/data",
  "tool-workspace": "/settings/tools",
  activity: "/settings/activity",
  "connect-panel": "/settings/accounts",
  settings: "/settings",
};

/** The three application destinations, shared by the workspace app and chat. */
export const PRODUCT_NAVIGATION = [
  {
    id: "home",
    label: "Home",
    href: "/",
    path: '<path d="m3 10 9-7 9 7v10H3Z"/><path d="M9 20v-7h6v7"/>',
  },
  {
    id: "chat",
    label: "Chat",
    href: "/chat",
    path: '<path d="M21 11.5a8.5 8.5 0 0 1-12.3 7.6L3 21l1.9-5.7A8.5 8.5 0 1 1 21 11.5Z"/>',
  },
  {
    id: "settings",
    label: "Settings",
    href: "/settings",
    path: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z"/>',
  },
] as const;
export type ProductNavId = (typeof PRODUCT_NAVIGATION)[number]["id"];

/** Which sidebar item a workspace path belongs to. */
export function productNavFor(path: string): ProductNavId {
  if (path === "/chat") return "chat";
  return path.startsWith("/settings") ? "settings" : "home";
}
