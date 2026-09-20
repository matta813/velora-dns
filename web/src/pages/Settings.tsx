import { useEffect, useState } from "react";
import { Save, AlertCircle, CheckCircle } from "lucide-react";
import type { Snapshot, Config } from "../api";
import {
  loadRateLimitSettings,
  saveConfig,
  saveRateLimitSettings,
  type RateLimitSettings,
} from "../api";
import { SUPPORTED_LANGUAGES, languageName, type Language } from "../i18n";
import { useI18n } from "../i18n-context";
import { themeLabel, SUPPORTED_THEMES, type Theme } from "../theme";
import { useTheme } from "../theme-context";

interface ConfigFormData {
  dns_listen: string;
  dns_upstreams: string;
  dns_allowed_clients: string;
  http_listen: string;
  http_allowed_hosts: string;
  query_log_enabled: boolean;
  log_level: string;
}

export function Settings({ data }: { data: Snapshot }) {
  const { t, language, setLanguage } = useI18n();
  const { theme, setTheme } = useTheme();
  const [formData, setFormData] = useState<ConfigFormData>({
    dns_listen: (data.config.dns.listen ?? ["127.0.0.1:53"]).join(", "),
    dns_upstreams: (data.config.dns.upstreams ?? ["1.1.1.1:53", "9.9.9.9:53"]).join(", "),
    dns_allowed_clients: (data.config.dns.allowed_clients ?? ["127.0.0.0/8", "::1/128"]).join(", "),
    http_listen: data.config.http.listen ?? "127.0.0.1:8080",
    http_allowed_hosts: (data.config.http.allowed_hosts ?? ["localhost", "127.0.0.1", "::1"]).join(", "),
    query_log_enabled: data.config.query_log.enabled ?? true,
    log_level: data.config.log_level ?? "info",
  });
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  const [rateLimit, setRateLimit] = useState<RateLimitSettings | null>(null);
  const [loadingRateLimit, setLoadingRateLimit] = useState(true);
  const [rateLimitError, setRateLimitError] = useState("");
  const [rateLimitSaved, setRateLimitSaved] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void loadRateLimitSettings(controller.signal)
      .then((settings) => {
        if (!controller.signal.aborted) {
          setRateLimit(settings);
          setRateLimitError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setRateLimitError(e instanceof Error ? e.message : t("settings.rate_limit_load_failed"));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingRateLimit(false);
      });
    return () => controller.abort();
  }, [t]);

  async function saveRateLimit() {
    if (!rateLimit) return;
    setRateLimitError("");
    setRateLimitSaved(false);
    try {
      const payload: RateLimitSettings = {
        ...rateLimit,
        global_qps: rateLimit.global_qps || 1,
        client_qps: rateLimit.client_qps || 1,
        rate_limit_burst: rateLimit.rate_limit_burst || 1,
      };
      const saved = await saveRateLimitSettings(payload);
      setRateLimit(saved);
      setRateLimitSaved(true);
    } catch (e) {
      setRateLimitError(e instanceof Error ? e.message : t("settings.rate_limit_save_failed"));
    }
  }

  const handleChange = <K extends keyof ConfigFormData>(key: K, value: ConfigFormData[K]) => {
    setFormData((prev) => ({ ...prev, [key]: value }));
    setMessage(null);
  };

  const parseList = (value: string): string[] =>
    value
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

  const handleSave = async () => {
    setSaving(true);
    setMessage(null);
    try {
      const newConfig: Config = {
        ...data.config,
        dns: {
          ...data.config.dns,
          listen: parseList(formData.dns_listen),
          upstreams: parseList(formData.dns_upstreams),
          allowed_clients: parseList(formData.dns_allowed_clients),
        },
        http: {
          ...data.config.http,
          listen: formData.http_listen,
          allowed_hosts: parseList(formData.http_allowed_hosts),
        },
        query_log: {
          ...data.config.query_log,
          enabled: formData.query_log_enabled,
        },
        log_level: formData.log_level,
      };
      await saveConfig(newConfig);
      setMessage({ type: "success", text: t("settings.save_success") });
    } catch (e) {
      setMessage({ type: "error", text: e instanceof Error ? e.message : t("settings.save_failed") });
    } finally {
      setSaving(false);
    }
  };

  const logLevelOptions = ["debug", "info", "warn", "error"];

  return (
    <>
      <section className="panel padded">
        <h2>{t("settings.server_configuration")}</h2>
        <p style={{ marginBottom: "1.5rem", opacity: 0.7 }}>
          {t("settings.edit_hint")}
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
            <h3 style={{ marginBottom: "1rem" }}>{t("settings.dns_section")}</h3>
            <div style={{ display: "grid", gap: "1rem" }}>
              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.dns_listen_label")}</span>
                <input
                  type="text"
                  value={formData.dns_listen}
                  onChange={(e) => handleChange("dns_listen", e.target.value)}
                  placeholder="127.0.0.1:53, [::1]:53"
                  style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
                />
                <small style={{ opacity: 0.6 }}>{t("settings.dns_listen_warning")}</small>
              </label>

              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.upstreams_label")}</span>
                <input
                  type="text"
                  value={formData.dns_upstreams}
                  onChange={(e) => handleChange("dns_upstreams", e.target.value)}
                  placeholder="1.1.1.1:53, 9.9.9.9:53"
                  style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
                />
                <small style={{ opacity: 0.6 }}>{t("settings.upstreams_hint")}</small>
              </label>

              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.allowed_clients_label")}</span>
                <input
                  type="text"
                  value={formData.dns_allowed_clients}
                  onChange={(e) => handleChange("dns_allowed_clients", e.target.value)}
                  placeholder="127.0.0.0/8, ::1/128"
                  style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
                />
                <small style={{ opacity: 0.6 }}>{t("settings.allowed_clients_hint")}</small>
              </label>
            </div>
          </div>

          <div className="panel" style={{ padding: "1rem" }}>
            <h3 style={{ marginBottom: "1rem" }}>{t("settings.web_section")}</h3>
            <div style={{ display: "grid", gap: "1rem" }}>
              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.web_listen_label")}</span>
                <input
                  type="text"
                  value={formData.http_listen}
                  onChange={(e) => handleChange("http_listen", e.target.value)}
                  placeholder="127.0.0.1:8080"
                  style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
                />
                <small style={{ opacity: 0.6 }}>{t("settings.web_listen_warning")}</small>
              </label>

              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.allowed_hosts_label")}</span>
                <input
                  type="text"
                  value={formData.http_allowed_hosts}
                  onChange={(e) => handleChange("http_allowed_hosts", e.target.value)}
                  placeholder="localhost, 127.0.0.1, ::1"
                  style={{ padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
                />
                <small style={{ opacity: 0.6 }}>{t("settings.allowed_hosts_hint")}</small>
              </label>
            </div>
          </div>

          <div className="panel" style={{ padding: "1rem" }}>
            <h3 style={{ marginBottom: "1rem" }}>{t("settings.logging_section")}</h3>
            <div style={{ display: "grid", gap: "1rem" }}>
              <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <input
                  type="checkbox"
                  checked={formData.query_log_enabled}
                  onChange={(e) => handleChange("query_log_enabled", e.target.checked)}
                />
                <span>{t("settings.enable_query_log")}</span>
              </label>

              <label style={{ display: "grid", gap: "0.5rem" }}>
                <span>{t("settings.log_level")}</span>
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
            {saving ? t("settings.saving") : t("settings.save_configuration")}
          </button>
          {message?.type === "success" && <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />}
        </div>
      </section>

      <section className="panel padded">
        <h2>{t("settings.rate_limiting")}</h2>
        <p>{t("settings.rate_limit_hint")}</p>
        {loadingRateLimit ? (
          <p role="status">{t("settings.rate_limit_loading")}</p>
        ) : rateLimit ? (
          <fieldset>
            <div className="form-grid">
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={rateLimit.enabled}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, enabled: e.target.checked })
                  }
                />
                {t("settings.rate_limit_enabled")}
              </label>
              <label>
                {t("settings.rate_limit_global_qps")}
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={rateLimit.global_qps || ""}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, global_qps: e.target.value === "" ? 0 : Number(e.target.value) })
                  }
                />
              </label>
              <label>
                {t("settings.rate_limit_client_qps")}
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={rateLimit.client_qps || ""}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, client_qps: e.target.value === "" ? 0 : Number(e.target.value) })
                  }
                />
              </label>
              <label>
                {t("settings.rate_limit_burst")}
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={rateLimit.rate_limit_burst || ""}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, rate_limit_burst: e.target.value === "" ? 0 : Number(e.target.value) })
                  }
                />
              </label>
            </div>
            {rateLimitError && (
              <p className="notice error" role="alert">{rateLimitError}</p>
            )}
            {rateLimitSaved && (
              <p className="notice success" role="status">{t("settings.rate_limit_saved")}</p>
            )}
            <div className="form-actions">
              <button className="button primary" onClick={() => void saveRateLimit()}>
                {t("settings.save")}
              </button>
            </div>
          </fieldset>
        ) : null}
      </section>

      <div className="notice">
        {t("settings.yaml_note")}
      </div>
    </>
  );
}
