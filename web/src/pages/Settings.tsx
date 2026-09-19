import { useEffect, useState } from "react";
import { APIError, type DeploymentSettings, loadDeploymentSettings, saveDeploymentSettings, type Snapshot } from "../api";
export function Settings({ data, admin }: { data: Snapshot; admin: boolean }) {
  const [settings, setSettings] = useState<DeploymentSettings>();
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!admin) return;
    const controller = new AbortController();
    void loadDeploymentSettings(controller.signal).then((result) => setSettings(result.settings)).catch((error: unknown) => setMessage(error instanceof APIError ? error.message : "Deployment controls are unavailable on this installation."));
    return () => controller.abort();
  }, [admin]);
  const rows: [string, string][] = [
    ["DNS listeners", data.status.dns_listen.join(", ")],
    ["Allowed client networks", data.config.dns.allowed_clients.join(", ")],
    ["Upstreams", data.config.dns.upstreams.join(", ")],
    ["Cache capacity", String(data.cache.capacity)],
    ["Log level", data.config.log_level],
    ["Version", data.status.version.version],
  ];
  return (
    <section className="panel padded">
      <h2>Server configuration</h2>
      {admin && settings && <form className="deployment-form" onSubmit={async (event) => { event.preventDefault(); setSaving(true); setMessage(""); try { await saveDeploymentSettings(settings); setMessage("Saved. Velora DNS is restarting with the selected settings."); } catch (error) { setMessage(error instanceof APIError ? error.message : "Could not save deployment settings."); } finally { setSaving(false); } }}>
        <h3>Deployment</h3><p>These controls change the local installation and restart Velora DNS. LAN exposure should only be used on trusted networks.</p>
        <label>Release channel<select value={settings.channel} onChange={(e) => setSettings({ ...settings, channel: e.target.value as DeploymentSettings["channel"] })}><option value="stable">Stable</option><option value="beta">Beta</option><option value="alpha">Alpha</option></select></label>
        <label>Web UI exposure<select value={settings.web_ui_exposure} onChange={(e) => setSettings({ ...settings, web_ui_exposure: e.target.value as DeploymentSettings["web_ui_exposure"] })}><option value="local">Localhost only</option><option value="lan">All interfaces (LAN)</option></select></label>
        <label>DNS exposure<select value={settings.dns_exposure} onChange={(e) => setSettings({ ...settings, dns_exposure: e.target.value as DeploymentSettings["dns_exposure"] })}><option value="local">Localhost only</option><option value="lan">Private LAN networks</option></select></label>
        <button className="button primary" disabled={saving}>{saving ? "Saving…" : "Apply and restart"}</button>
      </form>}
      {message && <div className="notice" role="status">{message}</div>}
      {!admin && <p>Deployment controls require an administrator account.</p>}
      <dl className="settings-list">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>
              <code>{value}</code>
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
