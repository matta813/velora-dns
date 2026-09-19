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
import { logout } from "./api";
import { useAuthUser } from "./auth-context";
export default function App() {
  const { data, error, history, refresh } = useSnapshot();
  const user = useAuthUser();
  const readOnly = user?.role === "viewer";
  const { pathname } = useLocation();
  const signOut = () => void logout().then(() => window.location.reload());
  const title =
    pathname === "/zones"
      ? "Local zones"
      : pathname === "/queries"
        ? "Query log"
        : pathname === "/blocklists"
          ? "Blocklists"
          : pathname === "/cache"
            ? "DNS cache"
            : pathname === "/settings"
              ? "Settings"
              : pathname === "/updates"
                ? "Updates"
                : pathname === "/backup"
                  ? "Backup & Restore"
                  : "Network overview";
  useEffect(() => {
    document.title = `Velora DNS · ${title}`;
  }, [title]);
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        Skip to content
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
            <strong>Local resolver</strong>
            <small>Self-hosted · Foundation</small>
          </div>
        </div>
        <span className="nav-label">WORKSPACE</span>
        <nav aria-label="Main navigation">
          <NavLink to="/" end>
            <LayoutDashboard size={18} />
            Overview
          </NavLink>
          <NavLink to="/zones">
            <Globe2 size={18} />
            Local zones
          </NavLink>
          <NavLink to="/queries">
            <ScrollText size={18} />
            Query log
          </NavLink>
          <NavLink to="/blocklists">
            <ShieldBan size={18} />
            Blocklists
          </NavLink>
          <NavLink to="/cache">
            <Database size={18} />
            DNS cache
          </NavLink>
          <NavLink to="/settings">
            <Settings2 size={18} />
            Settings
          </NavLink>
          <NavLink to="/updates">
            <Download size={18} />
            Updates
          </NavLink>
          <NavLink to="/backup">
            <HardDrive size={18} />
            Backup
          </NavLink>
        </nav>
        <button className="button secondary mobile-signout" onClick={signOut}>
          Sign out
        </button>
        <div className="sidebar-bottom">
          <div className="privacy">
            <ShieldCheck size={18} />
            <div>
              <strong>Private by default</strong>
              <p>
                {data?.config.query_log?.enabled
                  ? "Query history has bounded retention."
                  : "Query history is not collected."}
              </p>
            </div>
          </div>
          <a href="https://github.com/matta813/velora-dns">
            GitHub repository <ArrowUpRight size={15} />
          </a>
          <button className="button secondary" onClick={signOut}>Sign out</button>
          <small>{data?.status.version.version ?? "Connecting…"}</small>
        </div>
      </aside>
      <div className="main-wrap">
        <header className="topbar">
          <span>
            Workspace <span className="slash">/</span> <strong>{title}</strong>
          </span>
          <span className={`connection ${error ? "offline" : ""}`}>
            <i className="dot" />
            {error
              ? "Connection lost"
              : data?.status.ready
                ? "Resolver online"
                : "Connecting"}
          </span>
        </header>
        <main id="main">
          <div className="page-heading">
            <div>
              <span className="eyebrow">YOUR NETWORK, AT A GLANCE</span>
              <h1>{title}</h1>
              <p>
                {pathname === "/"
                  ? "A clear view of your DNS, from request to response."
                  : "Inspect and manage your resolver foundation."}
              </p>
            </div>
            {!["/zones", "/queries", "/blocklists"].includes(pathname) && (
              <button className="button" onClick={refresh}>
                <RefreshCw size={15} />
                Refresh
              </button>
            )}
          </div>
          {error && (
            <div className="notice error" role="alert">
              {error}.{" "}
              {data
                ? `Showing the last successful snapshot from ${data.checked.toLocaleTimeString()}.`
                : "Check that the Velora server is running."}
            </div>
          )}
          {readOnly && ["/zones", "/blocklists", "/cache"].includes(pathname) && (
            <div className="notice" role="status">
              You are signed in as a viewer. Management actions are read-only.
            </div>
          )}
          {!data && !error && (
            <div className="panel padded" role="status">
              Connecting to your resolver…
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
          <Route path="/settings" element={<Settings data={data} admin={user?.role === "admin"} />} />
              <Route path="/updates" element={<UpdateCenter readOnly={readOnly} />} />
              <Route path="/backup" element={<BackupAssistant readOnly={readOnly} />} />
              <Route path="*" element={<p>Page not found.</p>} />
            </Routes>
          )}
          <footer>
            <span>
              Velora DNS <span className="footer-dot">·</span> Independent.
              Self-hosted.
            </span>
            <span>
              {data
                ? `Last updated ${data.checked.toLocaleTimeString()}`
                : "Waiting for server"}
            </span>
          </footer>
        </main>
      </div>
    </div>
  );
}
