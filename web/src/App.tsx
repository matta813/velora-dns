import {
  Activity,
  Globe2,
  ArrowUpRight,
  Database,
  LayoutDashboard,
  RefreshCw,
  Settings2,
  ShieldCheck,
  ShieldBan,
  ScrollText,
  Download,
  HardDrive,
  Network,
} from "lucide-react";
import { useEffect } from "react";
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
import { logout } from "./api";
import { useAuthUser } from "./auth-context";
import { useI18n } from "./i18n-context";
export default function App() {
  const { data, error, history, refresh } = useSnapshot();
  const user = useAuthUser();
  const { t } = useI18n();
  const readOnly = user?.role === "viewer";
  const { pathname } = useLocation();
  const signOut = () => void logout().then(() => window.location.reload());
  const title =
    pathname === "/zones"
      ? t("app.title.zones")
      : pathname === "/queries"
        ? t("app.title.queries")
        : pathname === "/blocklists"
          ? t("app.title.blocklists")
          : pathname === "/cache"
            ? t("app.title.cache")
            : pathname === "/settings"
              ? t("app.title.settings")
              : pathname === "/updates"
                ? t("app.title.updates")
                : pathname === "/backup"
                  ? t("app.title.backup")
                  : pathname === "/dhcp"
                    ? t("app.title.dhcp")
                    : t("app.title.overview");
  useEffect(() => {
    document.title = `Velora DNS · ${title}`;
  }, [title]);
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        {t("app.skip_to_content")}
      </a>
      <aside className="sidebar">
        <a className="brand" href="/">
          <img src="/favicon.svg" width="38" height="38" alt="" />
          <span>
            velora<span className="brand-dns">DNS</span>
          </span>
        </a>
        <div className="workspace">
          <span className="workspace-icon">
            <Activity size={18} />
          </span>
          <div>
            <strong>{t("app.local_resolver")}</strong>
            <small>{t("app.self_hosted_foundation")}</small>
          </div>
        </div>
        <span className="nav-label">{t("app.workspace_label")}</span>
        <nav aria-label={t("app.nav.main")}>
          <NavLink to="/" end>
            <LayoutDashboard size={18} />
            {t("app.nav.overview")}
          </NavLink>
          <NavLink to="/zones">
            <Globe2 size={18} />
            {t("app.nav.zones")}
          </NavLink>
          <NavLink to="/queries">
            <ScrollText size={18} />
            {t("app.nav.queries")}
          </NavLink>
          <NavLink to="/blocklists">
            <ShieldBan size={18} />
            {t("app.nav.blocklists")}
          </NavLink>
          <NavLink to="/cache">
            <Database size={18} />
            {t("app.nav.cache")}
          </NavLink>
          <NavLink to="/settings">
            <Settings2 size={18} />
            {t("app.nav.settings")}
          </NavLink>
          <NavLink to="/updates">
            <Download size={18} />
            {t("app.nav.updates")}
          </NavLink>
          <NavLink to="/backup">
            <HardDrive size={18} />
            {t("app.nav.backup")}
          </NavLink>
          <NavLink to="/dhcp">
            <Network size={18} />
            {t("app.nav.dhcp")}
          </NavLink>
        </nav>
        <button className="button secondary mobile-signout" onClick={signOut}>
          {t("app.sign_out")}
        </button>
        <div className="sidebar-bottom">
          <div className="privacy">
            <ShieldCheck size={18} />
            <div>
              <strong>{t("app.private_by_default")}</strong>
              <p>
                {data?.config.query_log?.enabled
                  ? t("app.privacy.query_retention")
                  : t("app.privacy.no_query_history")}
              </p>
            </div>
          </div>
          <a href="https://github.com/matta813/velora-dns">
            {t("app.github_repo")} <ArrowUpRight size={15} />
          </a>
          <button className="button secondary" onClick={signOut}>{t("app.sign_out")}</button>
          <small>{data?.status.version.version ?? t("app.connecting")}</small>
        </div>
      </aside>
      <div className="main-wrap">
        <header className="topbar">
          <span>
            {t("app.workspace")} <span className="slash">/</span> <strong>{title}</strong>
          </span>
          <span className={`connection ${error ? "offline" : ""}`}>
            <i className="dot" />
            {error
              ? t("app.connection_lost")
              : data?.status.ready
                ? t("app.resolver_online")
                : t("app.connecting")}
          </span>
        </header>
        <main id="main">
          <div className="page-heading">
            <div>
              <span className="eyebrow">{t("app.eyebrow")}</span>
              <h1>{title}</h1>
              <p>
                {pathname === "/"
                  ? t("app.subtitle.overview")
                  : t("app.subtitle.default")}
              </p>
            </div>
            {!["/zones", "/queries", "/blocklists"].includes(pathname) && (
              <button className="button" onClick={refresh}>
                <RefreshCw size={15} />
                {t("app.refresh")}
              </button>
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
            <div className="notice" role="status">
              {t("app.viewer_readonly")}
            </div>
          )}
          {!data && !error && (
            <div className="panel padded" role="status">
              {t("app.connecting_panel")}
            </div>
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
                  />
                }
              />
              <Route
                path="/cache"
                element={<CachePage data={data} refresh={refresh} readOnly={readOnly} />}
              />
              <Route path="/zones" element={<Zones readOnly={readOnly} />} />
              <Route
                path="/queries"
                element={
                  <QueryLog enabled={data.config.query_log?.enabled ?? false} />
                }
              />
              <Route path="/blocklists" element={<Blocklists readOnly={readOnly} />} />
              <Route path="/settings" element={<Settings data={data} />} />
              <Route path="/updates" element={<UpdateCenter readOnly={readOnly} />} />
              <Route path="/backup" element={<BackupAssistant readOnly={readOnly} />} />
              <Route path="/dhcp" element={<DHCP readOnly={readOnly} />} />
              <Route path="*" element={<p>{t("app.not_found")}</p>} />
            </Routes>
          )}
          <footer>
            <span>
              {t("app.footer.independent")}
            </span>
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
