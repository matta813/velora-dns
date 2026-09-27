import {
  ArrowUpRight,
  Bell,
  ClipboardList,
  Database,
  Download,
  Globe2,
  HardDrive,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  Network,
  RefreshCw,
  ScrollText,
  Server,
  Settings2,
  ShieldBan,
  ShieldCheck,
  Stethoscope,
  Sun,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useEffect, useState } from "react";
import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { useSnapshot } from "./useSnapshot";
import { Dashboard } from "./pages/Dashboard";
import { CachePage } from "./pages/CachePage";
import { Settings } from "./pages/Settings";
import { Zones } from "./pages/Zones";
import { QueryLog } from "./pages/QueryLog";
import { Blocklists } from "./pages/Blocklists";
import { UpdateCenter } from "./pages/UpdateCenter";
import { BackupAssistant } from "./pages/BackupAssistant";
import { DHCP } from "./pages/DHCP";
import { Cluster } from "./pages/Cluster";
import { Diagnostics } from "./pages/Diagnostics";
import { AuditLog } from "./pages/AuditLog";
import { EventCenter } from "./pages/EventCenter";
import { Loading } from "./components/EmptyState";
import { logout, request, type SystemEventPage } from "./api";
import { useAuthUser } from "./auth-context";
import { useI18n } from "./i18n-context";
import { useTheme } from "./theme-context";
import { resolveTheme } from "./theme";

interface NavItem {
  to: string;
  key: string;
  icon: ReactNode;
  adminOnly?: boolean;
}

// Routes grouped the way operators think about them: watch, name, serve, maintain.
const NAV_GROUPS: { label: string; items: NavItem[] }[] = [
  {
    label: "app.nav.group_monitor",
    items: [
      { to: "/", key: "overview", icon: <LayoutDashboard size={18} /> },
      { to: "/queries", key: "queries", icon: <ScrollText size={18} /> },
      { to: "/events", key: "events", icon: <Bell size={18} /> },
    ],
  },
  {
    label: "app.nav.group_dns",
    items: [
      { to: "/zones", key: "zones", icon: <Globe2 size={18} /> },
      { to: "/blocklists", key: "blocklists", icon: <ShieldBan size={18} /> },
      { to: "/cache", key: "cache", icon: <Database size={18} /> },
    ],
  },
  {
    label: "app.nav.group_network",
    items: [
      { to: "/dhcp", key: "dhcp", icon: <Network size={18} /> },
      { to: "/cluster", key: "cluster", icon: <Server size={18} /> },
    ],
  },
  {
    label: "app.nav.group_system",
    items: [
      { to: "/settings", key: "settings", icon: <Settings2 size={18} /> },
      { to: "/updates", key: "updates", icon: <Download size={18} /> },
      { to: "/backup", key: "backup", icon: <HardDrive size={18} /> },
      { to: "/diagnostics", key: "diagnostics", icon: <Stethoscope size={18} /> },
      { to: "/audit", key: "audit", icon: <ClipboardList size={18} />, adminOnly: true },
    ],
  },
];

const ROUTE_KEYS: Record<string, string> = {
  "/": "overview",
  "/events": "events",
  "/zones": "zones",
  "/queries": "queries",
  "/blocklists": "blocklists",
  "/cache": "cache",
  "/settings": "settings",
  "/updates": "updates",
  "/backup": "backup",
  "/dhcp": "dhcp",
  "/cluster": "cluster",
  "/diagnostics": "diagnostics",
  "/audit": "audit",
};

// Pages that manage their own reload controls.
const SELF_REFRESHING = ["/zones", "/queries", "/blocklists"];

export default function App() {
  const { data, error, history, refresh } = useSnapshot();
  const user = useAuthUser();
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  const readOnly = user?.role === "viewer";
  const { pathname } = useLocation();
  const [unreadEvents, setUnreadEvents] = useState(0);
  const [navOpen, setNavOpen] = useState(false);
  const refreshEvents = useCallback(() => {
    void request<SystemEventPage>("/api/v1/events?limit=1")
      .then((page) => setUnreadEvents(page.unread_count))
      .catch(() => setUnreadEvents(0));
  }, []);
  useEffect(() => {
    if (!user) return;
    refreshEvents();
    const timer = window.setInterval(refreshEvents, 10000);
    return () => window.clearInterval(timer);
  }, [user, refreshEvents]);
  useEffect(() => {
    if (!navOpen) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setNavOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [navOpen]);
  const signOut = () => void logout().then(() => window.location.reload());
  const routeKey = ROUTE_KEYS[pathname] ?? "overview";
  const title = t(`app.title.${routeKey}`);
  const subtitle =
    routeKey === "overview" ? t("app.subtitle.overview") : t(`app.subtitle.${routeKey}`);
  useEffect(() => {
    document.title = `Velora DNS · ${title}`;
  }, [title]);
  const dark = resolveTheme(theme) === "dark";
  const connection = error
    ? { className: "offline", label: t("app.connection_lost") }
    : data?.status.ready
      ? { className: "", label: t("app.resolver_online") }
      : { className: "pending", label: t("app.connecting") };
  const closeNav = () => setNavOpen(false);

  return (
    <div className={`app${navOpen ? " nav-open" : ""}`}>
      <a className="skip-link" href="#main">
        {t("app.skip_to_content")}
      </a>
      <aside className="sidebar" id="sidebar">
        <div className="sidebar-header">
          <a className="app-brand" href="/">
            <img src="/favicon.svg" width="30" height="30" alt="" />
            <span>
              velora<span className="brand-dns">DNS</span>
            </span>
          </a>
          <button
            className="icon-button sidebar-close"
            onClick={closeNav}
            aria-label={t("app.menu_close")}
          >
            <X size={18} />
          </button>
        </div>
        <div className="sidebar-scroll">
          <nav aria-label={t("app.nav.main")}>
            {NAV_GROUPS.map((group) => {
              const items = group.items.filter(
                (item) => !item.adminOnly || user?.role === "admin",
              );
              return (
                <div className="nav-group" key={group.label}>
                  <span className="nav-label">{t(group.label)}</span>
                  <ul className="nav-list">
                    {items.map((item) => (
                      <li key={item.to}>
                        <NavLink to={item.to} end={item.to === "/"} onClick={closeNav}>
                          {item.icon}
                          {t(`app.nav.${item.key}`)}
                          {item.key === "events" && unreadEvents > 0 && (
                            <span className="event-count">{unreadEvents}</span>
                          )}
                        </NavLink>
                      </li>
                    ))}
                  </ul>
                </div>
              );
            })}
          </nav>
        </div>
        <div className="sidebar-footer">
          <div className="privacy">
            <ShieldCheck size={16} />
            <div>
              <strong>{t("app.private_by_default")}</strong>
              <p>
                {data?.config.query_log?.enabled
                  ? t("app.privacy.query_retention")
                  : t("app.privacy.no_query_history")}
              </p>
            </div>
          </div>
          {user && (
            <div className="user-card">
              <span className="avatar" aria-hidden="true">
                {user.username.slice(0, 1)}
              </span>
              <div className="user-meta">
                <strong title={`${t("app.signed_in_as")} ${user.username}`}>{user.username}</strong>
                <span>{user.role}</span>
              </div>
            </div>
          )}
          <button className="button signout" onClick={signOut}>
            <LogOut size={15} />
            {t("app.sign_out")}
          </button>
          <div className="sidebar-links">
            <a href="https://github.com/matta813/velora-dns">
              {t("app.github_repo")} <ArrowUpRight size={13} />
            </a>
          </div>
        </div>
      </aside>
      <div className="scrim" onClick={closeNav} aria-hidden="true" />
      <div className="main-wrap">
        <header className="topbar">
          <button
            className="icon-button menu-button"
            onClick={() => setNavOpen(true)}
            aria-label={t("app.menu_open")}
            aria-controls="sidebar"
            aria-expanded={navOpen}
          >
            <Menu size={20} />
          </button>
          <span className="breadcrumb">
            <span className="crumb-root">{t("app.workspace")}</span>
            <span className="slash">/</span>
            <strong>{title}</strong>
          </span>
          <div className="topbar-actions">
            <span className={`connection ${connection.className}`} title={connection.label}>
              <i className="dot" />
              <span className="connection-label">{connection.label}</span>
            </span>
            <button
              className="icon-button"
              onClick={() => setTheme(dark ? "light" : "dark")}
              aria-label={dark ? t("app.theme_to_light") : t("app.theme_to_dark")}
              title={dark ? t("app.theme_to_light") : t("app.theme_to_dark")}
            >
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            <NavLink
              to="/events"
              className="icon-button event-entry"
              aria-label={`${t("app.nav.events")}: ${unreadEvents} ${t("events.unread")}`}
            >
              <Bell size={18} />
              {unreadEvents > 0 && <span className="event-count">{unreadEvents}</span>}
            </NavLink>
          </div>
        </header>
        <main id="main">
          <div className="page-heading">
            <div>
              <h1>{title}</h1>
              <p>{subtitle}</p>
            </div>
            {!SELF_REFRESHING.includes(pathname) && (
              <div className="page-actions">
                <button className="button" onClick={refresh} aria-label={t("app.refresh")}>
                  <RefreshCw size={15} />
                  <span className="label">{t("app.refresh")}</span>
                </button>
              </div>
            )}
          </div>
          {error && (
            <div className="notice error" role="alert">
              {error}.{" "}
              {data
                ? `${t("app.error.showing_last")} ${data.checked.toLocaleTimeString()}.`
                : t("app.error.check_server")}
            </div>
          )}
          {readOnly && ["/zones", "/blocklists", "/cache"].includes(pathname) && (
            <div className="notice info" role="status">
              {t("app.viewer_readonly")}
            </div>
          )}
          {!data && !error && (
            <section className="panel">
              <Loading>{t("app.connecting_panel")}</Loading>
            </section>
          )}
          {data && (
            <Routes>
              <Route
                path="/"
                element={
                  <Dashboard
                    data={data}
                    history={history}
                    queryLoggingEnabled={data.config.query_log?.enabled ?? false}
                    readOnly={user?.role !== "admin"}
                    refresh={refresh}
                  />
                }
              />
              <Route path="/events" element={<EventCenter onRead={refreshEvents} />} />
              <Route
                path="/cache"
                element={<CachePage data={data} refresh={refresh} readOnly={readOnly} />}
              />
              <Route path="/zones" element={<Zones readOnly={readOnly} />} />
              <Route
                path="/queries"
                element={<QueryLog enabled={data.config.query_log?.enabled ?? false} />}
              />
              <Route path="/blocklists" element={<Blocklists readOnly={readOnly} />} />
              <Route path="/settings" element={<Settings data={data} />} />
              <Route path="/updates" element={<UpdateCenter readOnly={readOnly} />} />
              <Route
                path="/backup"
                element={<BackupAssistant readOnly={readOnly} canCreate={user?.role === "admin"} />}
              />
              <Route path="/dhcp" element={<DHCP readOnly={readOnly} />} />
              <Route path="/cluster" element={<Cluster />} />
              <Route path="/diagnostics" element={<Diagnostics />} />
              <Route path="/audit" element={<AuditLog />} />
              <Route path="*" element={<p>{t("app.not_found")}</p>} />
            </Routes>
          )}
          <footer>
            <span>{t("app.footer.independent")}</span>
            {data?.status.version?.version && (
              <span className="running-version" data-testid="running-version">
                Velora DNS {data.status.version.version}
              </span>
            )}
            <span>
              {data
                ? `${t("app.footer.last_updated")} ${data.checked.toLocaleTimeString()}`
                : t("app.footer.waiting")}
            </span>
          </footer>
        </main>
      </div>
    </div>
  );
}
