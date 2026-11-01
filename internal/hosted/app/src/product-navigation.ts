/** Canonical application destinations shared by the hosted settings shell and chat. */
const PRODUCT_NAVIGATION = [
  {
    id: "home",
    label: "Home",
    href: "/#home",
    path: '<path d="m3 10 9-7 9 7v10H3Z"/><path d="M9 20v-7h6v7"/>',
  },
  {
    id: "google-data",
    label: "Google data",
    href: "/#google-data",
    path: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M9 10v10"/>',
  },
  {
    id: "chat",
    label: "Chat",
    href: "/chat",
    path: '<path d="M21 11.5a8.5 8.5 0 0 1-12.3 7.6L3 21l1.9-5.7A8.5 8.5 0 1 1 21 11.5Z"/>',
  },
  {
    // No activity log exists yet, so the label names the real destination (N01).
    id: "activity",
    label: "Access",
    href: "/#activity",
    path: '<path d="M12 22s8-3.5 8-10V5l-8-3-8 3v7c0 6.5 8 10 8 10Z"/><path d="m9 12 2 2 4-5"/>',
  },
  {
    // The destination is the remote MCP connection, not workspace settings (N02).
    id: "settings",
    label: "Connect AI",
    href: "/#settings",
    path: '<path d="M12 22v-5M9 8V2M15 8V2"/><path d="M18 8v5a6 6 0 0 1-12 0V8Z"/>',
  },
] as const;

export function renderProductNavigation(active: string): string {
  return PRODUCT_NAVIGATION.map(
    (item) =>
      `<a href="${item.href}" data-product-nav="${item.id}"${active === item.id ? ' aria-current="page"' : ""}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${item.path}</svg><span>${item.label}</span></a>`,
  ).join("");
}
