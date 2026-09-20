import { useState } from "react";
import { Save, AlertCircle, CheckCircle } from "lucide-react";
import type { Snapshot, Config } from "../api";
import { saveConfig } from "../api";
import { SUPPORTED_LANGUAGES, languageName, type Language } from "../i18n";
import { useI18n } from "../i18n-context";
import { themeLabel, SUPPORTED_THEMES, type Theme } from "../theme";
import { useTheme } from "../theme-context";

interface ConfigFormData {
  dns_listen: string[];
  dns_upstreams: string[];
  dns_allowed_clients: string[];
  http_listen: string;
  http_allowed_hosts: string[];
  query_log_enabled: boolean;
  log_level: string;
}

export function Settings({ data }: { data: Snapshot }) {
  const { t, language, setLanguage } = useI18n();
  const { theme, setTheme } = useTheme();
  const [formData, setFormData] = useState<ConfigFormData>({
    dns_listen: data.config.dns.listen ?? ["127.0.0.1:53"],
    dns_upstreams: data.config.dns.upstreams ?? ["1.1.1.1:53", "9.9.9.9:53"],
    dns_allowed_clients: data.config.dns.allowed_clients ?? ["127.0.0.0/8", "::1/128"],
    http_listen: data.config.http.listen ?? "127.0.0.1:8080",
    http_allowed_hosts: data.config.http.allowed_hosts ?? ["localhost", "127.0.0.1", "::1"],
    query_log_enabled: data.config.query_log.enabled ?? true,
    log_level: data.config.log_level ?? "info",
  });
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  const handleChange = <K extends keyof ConfigFormData>(key: K, value: ConfigFormData[K]) => {
    setFormData((prev) => ({ ...prev, [key]: value }));
    setMessage(null);
  };

  const handleArrayChange = (key: keyof ConfigFormData, value: string) => {
    const arr = value.split(",").map((v) => v.trim()).filter((v) => v);
    handleChange(key, arr);
  };

  const handleSave = async () => {
    setSaving(true);
    setMessage(null);
    try {
      const newConfig: Config = {
        ...data.config,
        dns: {
          ...data.config.dns,
          listen: formData.dns_listen,
          upstreams: formData.dns_upstreams,
          allowed_clients: formData.dns_allowed_clients,
        },
        http: {
          ...data.config.http,
          listen: formData.http_listen,
          allowed_hosts: formData.http_allowed_hosts,
        },
        query_log: {
          ...data.config.query_log,
          enabled: formData.query_log_enabled,
        },
        log_level: formData.log_level,
      };
      await saveConfig(newConfig);
      setMessage({ type: "success", text: "Configuration saved. Restart required for changes to take effect." });
    } catch (e) {
      setMessage({ type: "error", text: e instanceof Error ? e.message : "Failed to save configuration" });
    } finally {
      setSaving(false);
    }
  };

  const dnsListenOptions = [
    { value: "127.0.0.1:53", label: "Localhost only (127.0.0.1:53)" },
    { value: "0.0.0.0:53", label: "All interfaces (0.0.0.0:53) — LAN access" },
  ];

  const httpListenOptions = [
    { value: "127.0.0.1:8080", label: "Localhost only (127.0.0.1:8080)" },
    { value: "0.0.0.0:8080", label: "All interfaces (0.0.0.0:8080) — LAN access" },
  ];

  const logLevelOptions = ["debug", "info", "warn", "error"];

  return (
    <section className="panel padded">
      <h2>{t("settings.server_configuration")}</h2>
      <p style={{ marginBottom: "1.5rem", opacity: 0.7 }}>
        These settings correspond to the quickstart installer options. Changes require a server restart to take effect.
      </p>

      <div className="form-grid">
        <label>
          {t("settings.language")}
          <select
            value={language}
            onChange={(e) => setLanguage(e.target.value as Language)}
          >
            {SUPPORTED_LANGUAGES.map((code) => (
              <option key={code} value={code}>
                {languageName(code)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("settings.theme")}
          <select
            value={theme}
            onChange={(e) => setTheme(e.target.value as Theme)}
          >
            {SUPPORTED_THEMES.map((code) => (
              <option key={code} value={code}>
                {themeLabel(code)}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="panel-footnote">{t("settings.theme_hint")}</p>

      {message && (
        <div className={`notice ${message.type === "error" ? "error" : ""}`} role="alert" style={{ marginBottom: "1rem" }}>
          {message.type === "error" ? <AlertCircle size={18} /> : <CheckCircle size={18} />}
          {message.text}
        </div>
      )}

      <div style={{ display: "grid", gap: "1.5rem" }}>
        <div className="panel" style={{ padding: "1rem" }}>
          <h3 style={{ marginBottom: "1rem" }}>DNS Settings</h3>
          <div style={{ display: "grid", gap: "1rem" }}>
            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>DNS Listen Address</span>
              <select
                value={formData.dns_listen[0] || "127.0.0.1:53"}
                onChange={(e) => handleChange("dns_listen", [e.target.value])}
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              >
                {dnsListenOptions.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              <small style={{ opacity: 0.6 }}>Warning: 0.0.0.0 exposes DNS to your network. Ensure firewall allows only trusted clients.</small>
            </label>

            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>Upstream DNS Servers (comma-separated)</span>
              <input
                type="text"
                value={formData.dns_upstreams.join(", ")}
                onChange={(e) => handleArrayChange("dns_upstreams", e.target.value)}
                placeholder="1.1.1.1:53, 9.9.9.9:53"
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              />
              <small style={{ opacity: 0.6 }}>Format: IP:PORT (e.g., 1.1.1.1:53). DoH upstreams use https://host/dns-query</small>
            </label>

            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>Allowed Client Networks (comma-separated CIDRs)</span>
              <input
                type="text"
                value={formData.dns_allowed_clients.join(", ")}
                onChange={(e) => handleArrayChange("dns_allowed_clients", e.target.value)}
                placeholder="127.0.0.0/8, ::1/128"
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              />
              <small style={{ opacity: 0.6 }}>Only these networks can query the resolver. Example: 192.168.1.0/24, 10.0.0.0/8</small>
            </label>
          </div>
        </div>

        <div className="panel" style={{ padding: "1rem" }}>
          <h3 style={{ marginBottom: "1rem" }}>Web UI Settings</h3>
          <div style={{ display: "grid", gap: "1rem" }}>
            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>Web UI Listen Address</span>
              <select
                value={formData.http_listen}
                onChange={(e) => handleChange("http_listen", e.target.value)}
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              >
                {httpListenOptions.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              <small style={{ opacity: 0.6 }}>Warning: 0.0.0.0 exposes the Web UI to your network. Use a firewall or authentication.</small>
            </label>

            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>Allowed Hosts (comma-separated)</span>
              <input
                type="text"
                value={formData.http_allowed_hosts.join(", ")}
                onChange={(e) => handleArrayChange("http_allowed_hosts", e.target.value)}
                placeholder="localhost, 127.0.0.1, ::1"
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              />
              <small style={{ opacity: 0.6 }}>Host headers allowed to access the management API. Use * to allow all (not recommended for LAN).</small>
            </label>
          </div>
        </div>

        <div className="panel" style={{ padding: "1rem" }}>
          <h3 style={{ marginBottom: "1rem" }}>Logging & Other</h3>
          <div style={{ display: "grid", gap: "1rem" }}>
            <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <input
                type="checkbox"
                checked={formData.query_log_enabled}
                onChange={(e) => handleChange("query_log_enabled", e.target.checked)}
              />
              <span>Enable Query Logging</span>
            </label>

            <label style={{ display: "grid", gap: "0.5rem" }}>
              <span>Log Level</span>
              <select
                value={formData.log_level}
                onChange={(e) => handleChange("log_level", e.target.value)}
                style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
              >
                {logLevelOptions.map((level) => (
                  <option key={level} value={level}>{level}</option>
                ))}
              </select>
            </label>
          </div>
        </div>
      </div>

      <div style={{ marginTop: "1.5rem", display: "flex", gap: "1rem" }}>
        <button
          className="button primary"
          onClick={handleSave}
          disabled={saving}
          style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}
        >
          <Save size={18} />
          {saving ? "Saving..." : "Save Configuration"}
        </button>
        {message?.type === "success" && <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />}
      </div>

      <div className="notice" style={{ marginTop: "1rem" }}>
        <strong>Note:</strong> Configuration is saved to the YAML config file. The server must be restarted for changes to take effect.
        The installer also manages release channel (stable/beta/alpha) and bootstrap credentials separately via environment files.
      </div>
    </section>
  );
}