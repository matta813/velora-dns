import {
  Activity,
  ArrowUpRight,
  Database,
  LayoutDashboard,
  RefreshCw,
  Settings2,
  ShieldCheck,
} from "lucide-react";
import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { useSnapshot } from "./useSnapshot";
import { Dashboard } from "./pages/Dashboard";
import { CachePage } from "./pages/CachePage";
import { Settings } from "./pages/Settings";
export default function App() {
  const { data, error, history, refresh } = useSnapshot();
  const { pathname } = useLocation();
  const title =
    pathname === "/cache"
      ? "DNS cache"
      : pathname === "/settings"
        ? "Settings"
        : "Network overview";
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
          <NavLink to="/cache">
            <Database size={18} />
            DNS cache
          </NavLink>
          <NavLink to="/settings">
            <Settings2 size={18} />
            Settings
          </NavLink>
        </nav>
        <div className="sidebar-bottom">
          <div className="privacy">
            <ShieldCheck size={18} />
            <div>
              <strong>Private by default</strong>
              <p>Query history is not collected.</p>
            </div>
          </div>
          <a href="https://github.com/matta813/velora-dns">
            GitHub repository <ArrowUpRight size={15} />
          </a>
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
            <button className="button" onClick={refresh}>
              <RefreshCw size={15} />
              Refresh
            </button>
          </div>
          {error && (
            <div className="notice error" role="alert">
              {error}.{" "}
              {data
                ? `Showing the last successful snapshot from ${data.checked.toLocaleTimeString()}.`
                : "Check that the Velora server is running."}
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
                element={<Dashboard data={data} history={history} />}
              />
              <Route
                path="/cache"
                element={<CachePage data={data} refresh={refresh} />}
              />
              <Route path="/settings" element={<Settings data={data} />} />
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
