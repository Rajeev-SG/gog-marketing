import { useSyncExternalStore } from "react";
import { LEGACY_WORKSPACE_ANCHORS, WORKSPACE_PATHS } from "../product-navigation.js";

/** Router: real pages for the workspace, full-page navigation for everything else (chat, help). */

export type ClerkLike = {
  loaded?: boolean;
  session?: { id: string } | null;
  mountUserButton?: (el: HTMLElement) => void;
  signOut?: (options?: { redirectUrl?: string }) => Promise<void>;
};
export const clerk = () => (window as typeof window & { Clerk?: ClerkLike }).Clerk;

const listeners = new Set<() => void>();
const subscribe = (fn: () => void) => {
  listeners.add(fn);
  window.addEventListener("popstate", fn);
  return () => {
    listeners.delete(fn);
    window.removeEventListener("popstate", fn);
  };
};
const location = () => window.location.pathname + window.location.search + window.location.hash;
export function useLocation(): URL {
  const href = useSyncExternalStore(subscribe, location);
  return new URL(href, window.location.origin);
}
export const isWorkspacePath = (path: string) =>
  (WORKSPACE_PATHS as readonly string[]).includes(path);
export function navigate(href: string, replace = false) {
  const url = new URL(href, window.location.origin);
  if (url.origin !== window.location.origin || !isWorkspacePath(url.pathname)) {
    window.location.assign(url.href);
    return;
  }
  window.history[replace ? "replaceState" : "pushState"](
    null,
    "",
    url.pathname + url.search + url.hash,
  );
  listeners.forEach((fn) => fn());
  if (url.hash)
    requestAnimationFrame(() =>
      document.getElementById(url.hash.slice(1))?.scrollIntoView({ block: "start" }),
    );
  else window.scrollTo({ top: 0 });
}

/** Old anchors still land on the right page: /#google-data, and /data#tools (now /settings/data#tools). */
export function redirectLegacyAnchor() {
  const { pathname, search, hash } = window.location;
  if (pathname === "/settings/data" && (hash === "#tools" || hash === "#tool-workspace")) {
    navigate("/settings/tools" + search, true);
    return;
  }
  const anchor = pathname === "/" ? hash.slice(1) : "";
  const target = anchor ? LEGACY_WORKSPACE_ANCHORS[anchor] : undefined;
  if (target && target !== "/") navigate(target, true);
}
