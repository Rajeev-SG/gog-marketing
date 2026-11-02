import { StrictMode, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Link, RouterProvider } from "react-aria-components";
import { PRODUCT_CSS } from "../product-css.generated.js";
import { PRODUCT_NAVIGATION } from "../product-navigation.js";
import { clerk, isWorkspacePath, navigate, redirectLegacyAnchor, useLocation } from "./router.js";
import { ConfirmProvider, Icon, Notice } from "../ui/index.js";
import { WorkspaceProvider, useWorkspace } from "./state.js";
import { HomePage } from "./pages/HomePage.js";
import { DataPage } from "./pages/DataPage.js";
import { AccessPage } from "./pages/AccessPage.js";
import { ConnectAiPage } from "./pages/ConnectAiPage.js";
import "./workspace.css";

const NAV_IDS: Record<string, string> = {
  "/": "home",
  "/data": "google-data",
  "/access": "activity",
  "/connect-ai": "settings",
};
const TITLES: Record<string, string> = {
  "/": "Home",
  "/data": "Google data",
  "/access": "Access",
  "/connect-ai": "Connect AI",
};

function Navigation({
  active,
  label,
  className,
}: {
  active: string;
  label: string;
  className: string;
}) {
  return (
    <nav className={className} aria-label={label}>
      {PRODUCT_NAVIGATION.map((item) => (
        <Link
          key={item.id}
          href={item.href}
          aria-current={active === item.id ? "page" : undefined}
          data-product-nav={item.id}
        >
          <Icon path={item.path} />
          <span>{item.label}</span>
        </Link>
      ))}
    </nav>
  );
}

function UserButton() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let cancelled = false;
    const mount = () => {
      const c = clerk();
      if (cancelled) return;
      if (c?.loaded && ref.current && c.mountUserButton) c.mountUserButton(ref.current);
      else setTimeout(mount, 150);
    };
    mount();
    return () => {
      cancelled = true;
    };
  }, []);
  return <div ref={ref} className="user-button" />;
}

function Shell() {
  const url = useLocation();
  const path = isWorkspacePath(url.pathname) ? url.pathname : "/";
  const workspace = useWorkspace();
  const [query, setQuery] = useState("");
  const main = useRef<HTMLElement>(null);
  const firstRender = useRef(true);

  useEffect(redirectLegacyAnchor, []);
  // Phones use the bottom bar; the off-canvas sidebar must not take focus (N03, L13).
  const sidebar = useRef<HTMLElement>(null);
  useEffect(() => {
    const query = window.matchMedia("(max-width: 900px)");
    const update = () => {
      if (sidebar.current) sidebar.current.inert = query.matches;
    };
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  useEffect(() => {
    document.title = `${TITLES[path]} — gog-marketing`;
    // Move focus to the new page so screen readers announce it (route change).
    if (firstRender.current) firstRender.current = false;
    else main.current?.focus({ preventScroll: true });
  }, [path]);

  const signOut = () => {
    const c = clerk();
    if (!c?.signOut) return;
    c.signOut({ redirectUrl: "/" }).catch(() =>
      workspace.setNotice({
        tone: "warning",
        text: "Could not complete sign-out. Your workspace remains open; retry when the sign-in service is available.",
      }),
    );
  };

  return (
    <div className="app-shell" id="app-shell">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <aside className="sidebar" id="app-sidebar" aria-label="Application navigation" ref={sidebar}>
        <Link className="sidebar-brand" href="/">
          <span className="brand-mark" aria-hidden="true">
            g
          </span>
          <span>gog-marketing</span>
        </Link>
        <div className="sidebar-rule" />
        <Navigation active={NAV_IDS[path]} label="Primary" className="side-nav" />
        <div className="sidebar-spacer" />
        <div className="trust-card">
          <Icon name="shield" />
          <strong>Secure by design</strong>
          <p>Least-privilege access to the Google data you choose.</p>
        </div>
      </aside>
      <div className="app-frame">
        <header className="topbar">
          <form
            className="topbar-search"
            role="search"
            onSubmit={(event) => {
              event.preventDefault();
              navigate("/data?q=" + encodeURIComponent(query.trim()));
            }}
          >
            <Icon name="search" />
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Find a file or resource"
              aria-label="Find a file or resource"
            />
          </form>
          <div className="topbar-actions">
            <a className="ui-button secondary" href="/about#help">
              <Icon name="help" /> Help
            </a>
            <button type="button" className="ui-button secondary" onClick={signOut}>
              Sign out
            </button>
            <UserButton />
          </div>
        </header>
        <main className="app-main" id="main-content" tabIndex={-1} ref={main}>
          {workspace.notice && (
            <Notice tone={workspace.notice.tone} onDismiss={() => workspace.setNotice(null)}>
              {workspace.notice.text}
            </Notice>
          )}
          {path === "/" && <HomePage />}
          {path === "/data" && <DataPage />}
          {path === "/access" && <AccessPage />}
          {path === "/connect-ai" && <ConnectAiPage />}
        </main>
        <footer className="app-footer">
          <span>gog-marketing · Secure access layer for Google marketing data</span>
          <a href="/about">Help &amp; privacy</a>
        </footer>
      </div>
      <Navigation active={NAV_IDS[path]} label="Mobile navigation" className="bottom-nav" />
    </div>
  );
}

function App() {
  return (
    <RouterProvider navigate={(href) => navigate(String(href))}>
      <ConfirmProvider>
        <WorkspaceProvider>
          <Shell />
        </WorkspaceProvider>
      </ConfirmProvider>
    </RouterProvider>
  );
}

const style = document.createElement("style");
style.textContent = PRODUCT_CSS;
document.head.prepend(style);
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
