/** Canonical application destinations shared by the hosted settings shell and chat. */
export const PRODUCT_NAVIGATION = [
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
    id: "activity",
    label: "Activity",
    href: "/#activity",
    path: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.5 2"/>',
  },
  {
    id: "settings",
    label: "Settings",
    href: "/#settings",
    path: '<circle cx="12" cy="12" r="3"/><path d="m9 3-1 3-3 1-2 4 2 3 1 4 4 3 4-1 4-2 3-4-1-4-2-4-4-1Z"/>',
  },
] as const;

export function renderProductNavigation(active: string): string {
  return PRODUCT_NAVIGATION.map(
    (item) =>
      `<a href="${item.href}" data-product-nav="${item.id}"${active === item.id ? ' aria-current="page"' : ""}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${item.path}</svg><span>${item.label}</span></a>`,
  ).join("");
}
