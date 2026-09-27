import {
  BarChart3,
  Bell,
  ChevronDown,
  ClipboardList,
  Database,
  Download,
  Globe2,
  HardDrive,
  LayoutDashboard,
  LogOut,
  Menu,
  MonitorSmartphone,
  Moon,
  Network,
  RefreshCw,
  Replace,
  Route as RouteIcon,
  ScrollText,
  Server,
  Settings2,
  ShieldBan,
  ShieldCheck,
  Webhook as WebhookIcon,
  Stethoscope,
  Sun,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { useSnapshot } from "./useSnapshot";
import { Dashboard } from "./pages/Dashboard";
import { CachePage } from "./pages/CachePage";
import { Settings } from "./pages/Settings";
import { Zones } from "./pages/Zones";
import { QueryLog } from "./pages/QueryLog";
import { Blocklists } from "./pages/Blocklists";
import { Forwarding } from "./pages/Forwarding";
import { Rewrites } from "./pages/Rewrites";
import { Clients } from "./pages/Clients";
import { Policies } from "./pages/Policies";
import { UpdateCenter } from "./pages/UpdateCenter";
import { BackupAssistant } from "./pages/BackupAssistant";
import { DHCP } from "./pages/DHCP";
import { Cluster } from "./pages/Cluster";
import { Diagnostics } from "./pages/Diagnostics";
import { AuditLog } from "./pages/AuditLog";
import { Analytics } from "./pages/Analytics";
import { Webhooks } from "./pages/Webhooks";
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

interface NavGroup {
  label: string;
  items: NavItem[];
}

// Top-level links first, then dropdown groups, the way most self-hosted
// network appliances lay out their admin navigation.
const NAV: (NavItem | NavGroup)[] = [
  { to: "/", key: "overview", icon: <LayoutDashboard size={17} /> },
  { to: "/queries", key: "queries", icon: <ScrollText size={17} /> },
  { to: "/analytics", key: "analytics", icon: <BarChart3 size={17} /> },
  {
    label: "app.nav.group_dns",
    items: [
      { to: "/zones", key: "zones", icon: <Globe2 size={16} /> },
      { to: "/blocklists", key: "blocklists", icon: <ShieldBan size={16} /> },
      { to: "/policies", key: "policies", icon: <ShieldCheck size={16} /> },
      { to: "/rewrites", key: "rewrites", icon: <Replace size={16} /> },
      { to: "/forwarding", key: "forwarding", icon: <RouteIcon size={16} /> },
      { to: "/cache", key: "cache", icon: <Database size={16} /> },
    ],
  },
  {
    label: "app.nav.group_network",
    items: [
      { to: "/clients", key: "clients", icon: <MonitorSmartphone size={16} /> },
      { to: "/dhcp", key: "dhcp", icon: <Network size={16} /> },
      { to: "/cluster", key: "cluster", icon: <Server size={16} /> },
    ],
  },
  {
    label: "app.nav.group_system",
    items: [
      { to: "/settings", key: "settings", icon: <Settings2 size={16} /> },
      { to: "/updates", key: "updates", icon: <Download size={16} /> },
      { to: "/backup", key: "backup", icon: <HardDrive size={16} /> },
      { to: "/diagnostics", key: "diagnostics", icon: <Stethoscope size={16} /> },
      { to: "/events", key: "events", icon: <Bell size={16} /> },
      { to: "/webhooks", key: "webhooks", icon: <WebhookIcon size={16} />, adminOnly: true },
      { to: "/audit", key: "audit", icon: <ClipboardList size={16} />, adminOnly: true },
    ],
  },
];

const GROUP_ICONS: Record<string, ReactNode> = {
  "app.nav.group_dns": <Globe2 size={17} />,
  "app.nav.group_network": <Network size={17} />,
  "app.nav.group_system": <Settings2 size={17} />,
};

const ROUTE_KEYS: Record<string, string> = {
  "/": "overview",
  "/events": "events",
  "/zones": "zones",
  "/queries": "queries",
  "/analytics": "analytics",
  "/blocklists": "blocklists",
  "/forwarding": "forwarding",
  "/rewrites": "rewrites",
  "/clients": "clients",
  "/policies": "policies",
  "/cache": "cache",
  "/settings": "settings",
  "/updates": "updates",
  "/backup": "backup",
  "/dhcp": "dhcp",
  "/cluster": "cluster",
  "/diagnostics": "diagnostics",
  "/audit": "audit",
  "/webhooks": "webhooks",
};

// Pages that manage their own reload controls.
const SELF_REFRESHING = ["/zones", "/queries", "/blocklists", "/forwarding", "/rewrites", "/clients", "/policies", "/webhooks", "/analytics", "/updates"];

export default function App() {
  const { data, error, history, refresh } = useSnapshot();
  const user = useAuthUser();
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  const readOnly = user?.role === "viewer";
  const { pathname } = useLocation();
  const [unreadEvents, setUnreadEvents] = useState(0);
  const [menuOpen, setMenuOpen] = useState(false);
  const [openGroup, setOpenGroup] = useState<string | null>(null);
  const navRef = useRef<HTMLElement>(null);
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
    if (!openGroup) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpenGroup(null);
    };
    const onPointer = (event: MouseEvent) => {
      if (!navRef.current?.contains(event.target as Node)) setOpenGroup(null);
    };
    window.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onPointer);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onPointer);
    };
  }, [openGroup]);
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
  const closeNav = () => {
    setOpenGroup(null);
    setMenuOpen(false);
  };

  return (
    <div className="app">
      <a className="skip-link" href="#main">
        {t("app.skip_to_content")}
      </a>
      <header className="header">
        <div className="container header-inner">
          <a className="app-brand" href="/">
            <img src="/favicon.svg" width="28" height="28" alt="" />
            <span>
              Velora <span>DNS</span>
            </span>
          </a>
          <div className="header-right">
            <span className={`connection ${connection.className}`} title={connection.label}>
              <i className="dot" />
              <span className="connection-label">{connection.label}</span>
            </span>
            <NavLink
              to="/events"
              className="icon-button event-entry"
              aria-label={`${t("app.nav.events")}: ${unreadEvents} ${t("events.unread")}`}
              onClick={closeNav}
            >
              <Bell size={18} />
              {unreadEvents > 0 && <span className="event-count">{unreadEvents}</span>}
            </NavLink>
            <button
              className="icon-button"
              onClick={() => setTheme(dark ? "light" : "dark")}
              aria-label={dark ? t("app.theme_to_light") : t("app.theme_to_dark")}
              title={dark ? t("app.theme_to_light") : t("app.theme_to_dark")}
            >
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            <div className="header-user">
              {user && (
                <div className="header-user-name" title={`${t("app.signed_in_as")} ${user.username}`}>
                  <strong>{user.username}</strong>
                  <span>{user.role}</span>
                </div>
              )}
              <button className="button small signout" onClick={signOut} title={t("app.sign_out")}>
                <LogOut size={15} />
                <span className="label">{t("app.sign_out")}</span>
              </button>
            </div>
            <button
              className="icon-button menu-button"
              onClick={() => setMenuOpen((open) => !open)}
              aria-label={menuOpen ? t("app.menu_close") : t("app.menu_open")}
              aria-controls="main-nav"
              aria-expanded={menuOpen}
            >
              {menuOpen ? <X size={20} /> : <Menu size={20} />}
            </button>
          </div>
        </div>
      </header>
      <nav
        id="main-nav"
        ref={navRef}
        className={`navbar${menuOpen ? " open" : ""}`}
        aria-label={t("app.nav.main")}
      >
        <div className="container">
          <ul className="nav-tabs">
            {NAV.map((entry) => {
              if ("to" in entry) {
                return (
                  <li key={entry.to}>
                    <NavLink to={entry.to} end={entry.to === "/"} className="nav-tab" onClick={closeNav}>
                      {entry.icon}
                      {t(`app.nav.${entry.key}`)}
                    </NavLink>
                  </li>
                );
              }
              const items = entry.items.filter((item) => !item.adminOnly || user?.role === "admin");
              const active = items.some((item) => item.to === pathname);
              const open = openGroup === entry.label || menuOpen;
              const menuId = `menu-${entry.label.split(".").pop()}`;
              return (
                <li key={entry.label}>
                  <button
                    className={`nav-tab group-toggle${active ? " active" : ""}`}
                    aria-expanded={open}
                    aria-controls={menuId}
                    onClick={() => setOpenGroup(openGroup === entry.label ? null : entry.label)}
                  >
                    {GROUP_ICONS[entry.label]}
                    {t(entry.label)}
                    <ChevronDown size={14} className="chevron" />
                  </button>
                  {open && (
                    <ul className="dropdown-menu" id={menuId}>
                      {items.map((item) => (
                        <li key={item.to}>
                          <NavLink to={item.to} onClick={closeNav}>
                            {item.icon}
                            {t(`app.nav.${item.key}`)}
                            {item.key === "events" && unreadEvents > 0 && (
                              <span className="event-count">{unreadEvents}</span>
                            )}
                          </NavLink>
                        </li>
                      ))}
                    </ul>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      </nav>
      <main id="main" className="container">
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
          {readOnly && ["/zones", "/blocklists", "/cache", "/forwarding", "/rewrites", "/clients", "/policies"].includes(pathname) && (
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
              <Route path="/policies" element={<Policies readOnly={readOnly} />} />
              <Route path="/clients" element={<Clients readOnly={readOnly} queryLogging={data.config.query_log?.enabled ?? false} />} />
              <Route path="/rewrites" element={<Rewrites readOnly={readOnly} />} />
              <Route path="/forwarding" element={<Forwarding readOnly={readOnly} />} />
              <Route path="/settings" element={<Settings data={data} />} />
              <Route path="/updates" element={<UpdateCenter readOnly={readOnly} onUpdated={refresh} />} />
              <Route
                path="/backup"
                element={<BackupAssistant readOnly={readOnly} canCreate={user?.role === "admin"} />}
              />
              <Route path="/dhcp" element={<DHCP readOnly={readOnly} />} />
              <Route path="/cluster" element={<Cluster />} />
              <Route path="/diagnostics" element={<Diagnostics />} />
              <Route path="/audit" element={<AuditLog />} />
              <Route path="/analytics" element={<Analytics queryLogging={data.config.query_log?.enabled ?? false} />} />
              <Route path="/webhooks" element={<Webhooks />} />
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
  );
}
