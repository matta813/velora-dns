import { useEffect, useState } from "react";
import { Save, AlertCircle, CheckCircle, ListChecks } from "lucide-react";
import type { Snapshot, Config, FieldError } from "../api";
import {
  APIError,
  validateConfig,
  loadRateLimitSettings,
  loadRateLimitStatus,
  saveConfig,
  saveRateLimitSettings,
  type RateLimitSettings,
  type RateLimitStatus,
} from "../api";
import { SUPPORTED_LANGUAGES, languageName, type Language } from "../i18n";
import { useI18n } from "../i18n-context";
import { SUPPORTED_THEMES, type Theme } from "../theme";
import { useTheme } from "../theme-context";
import { Loading } from "../components/EmptyState";

interface ConfigFormData {
  dns_listen: string;
  dns_upstreams: string;
  dns_allowed_clients: string;
  cache_upstream_ttl: string;
  http_listen: string;
  http_allowed_hosts: string;
  query_log_enabled: boolean;
  log_level: string;
}

type FormKey = keyof ConfigFormData;

// API field paths (without list indexes) that the form edits directly.
const FIELD_KEYS: Record<string, FormKey> = {
  "dns.listen": "dns_listen",
  "dns.upstreams": "dns_upstreams",
  "dns.allowed_clients": "dns_allowed_clients",
  "cache.upstream_ttl": "cache_upstream_ttl",
  "http.listen": "http_listen",
  "http.allowed_hosts": "http_allowed_hosts",
  "query_log.enabled": "query_log_enabled",
  "log_level": "log_level",
};

/** Groups API field errors by form input; list entries are numbered from 1. */
function groupFieldErrors(fields: FieldError[], entryLabel: string) {
  const out: Partial<Record<FormKey, string[]>> = {};
  for (const item of fields) {
    const match = /^([a-z_.]+)(?:\[(\d+)\])?$/.exec(item.field);
    const key = match ? FIELD_KEYS[match[1]] : undefined;
    if (!key) continue;
    const text = match?.[2] !== undefined ? `${entryLabel} ${Number(match[2]) + 1}: ${item.message}` : item.message;
    (out[key] ??= []).push(text);
  }
  return out;
}

export function Settings({ data }: { data: Snapshot }) {
  const { t, language, setLanguage } = useI18n();
  const { theme, setTheme } = useTheme();
  const [formData, setFormData] = useState<ConfigFormData>({
    dns_listen: (data.config.dns.listen ?? ["127.0.0.1:53"]).join(", "),
    dns_upstreams: (data.config.dns.upstreams ?? ["1.1.1.1:53", "9.9.9.9:53"]).join(", "),
    dns_allowed_clients: (data.config.dns.allowed_clients ?? ["127.0.0.0/8", "::1/128"]).join(", "),
    cache_upstream_ttl: String(data.config.cache.upstream_ttl ?? 86400),
    http_listen: data.config.http.listen ?? "127.0.0.1:8080",
    http_allowed_hosts: (data.config.http.allowed_hosts ?? ["localhost", "127.0.0.1", "::1"]).join(", "),
    query_log_enabled: data.config.query_log.enabled ?? true,
    log_level: data.config.log_level ?? "info",
  });
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<FormKey, string[]>>>({});

  const [rateLimit, setRateLimit] = useState<RateLimitSettings | null>(null);
  const [loadingRateLimit, setLoadingRateLimit] = useState(true);
  const [rateLimitError, setRateLimitError] = useState("");
  const [rateLimitSaved, setRateLimitSaved] = useState(false);
  const [rateLimitStatus, setRateLimitStatus] = useState<RateLimitStatus | null>(null);
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

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const status = await loadRateLimitStatus(controller.signal);
        if (!controller.signal.aborted && Number.isFinite(status.rejected_total)) setRateLimitStatus(status);
      } catch {
        if (!controller.signal.aborted) setRateLimitStatus(null);
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
      }
    };
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);

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
    setFieldErrors((prev) => ({ ...prev, [key]: undefined }));
  };

  const parseList = (value: string): string[] =>
    value
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);

  const buildConfig = (): Config | null => {
    const ttl = Number(formData.cache_upstream_ttl);
    if (!/^\d+$/.test(formData.cache_upstream_ttl.trim()) || !Number.isInteger(ttl) || ttl > 604800) {
      setMessage({ type: "error", text: t("settings.cache_ttl_invalid") });
      setFieldErrors({ cache_upstream_ttl: [t("settings.cache_ttl_invalid")] });
      return null;
    }
    return {
      ...data.config,
      dns: {
        ...data.config.dns,
        listen: parseList(formData.dns_listen),
        upstreams: parseList(formData.dns_upstreams),
        allowed_clients: parseList(formData.dns_allowed_clients),
      },
      cache: {
        ...data.config.cache,
        upstream_ttl: ttl,
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
  };

  const showFieldErrors = (fields: FieldError[]) => {
    setFieldErrors(groupFieldErrors(fields, t("settings.entry")));
  };

  const handleSave = async () => {
    const newConfig = buildConfig();
    if (!newConfig) return;
    setSaving(true);
    setMessage(null);
    setFieldErrors({});
    try {
      await saveConfig(newConfig);
      setMessage({ type: "success", text: t("settings.save_success") });
    } catch (e) {
      if (e instanceof APIError && e.fields.length) {
        showFieldErrors(e.fields);
        setMessage({ type: "error", text: e.code === "config_requires_restart" ? t("settings.restart_required") : t("settings.fix_fields") });
      } else {
        setMessage({ type: "error", text: e instanceof Error ? e.message : t("settings.save_failed") });
      }
    } finally {
      setSaving(false);
    }
  };

  const handleCheck = async () => {
    const candidate = buildConfig();
    if (!candidate) return;
    setSaving(true);
    setMessage(null);
    setFieldErrors({});
    try {
      const result = await validateConfig(candidate);
      if (result.valid) {
        setMessage({ type: "success", text: t("settings.check_ok") });
      } else if (result.errors.length) {
        showFieldErrors(result.errors);
        setMessage({ type: "error", text: t("settings.fix_fields") });
      } else {
        showFieldErrors(result.restart_required.map((field) => ({ field, message: t("settings.restart_field") })));
        setMessage({ type: "error", text: t("settings.restart_required") });
      }
    } catch (e) {
      setMessage({ type: "error", text: e instanceof Error ? e.message : t("settings.save_failed") });
    } finally {
      setSaving(false);
    }
  };

  const errorsFor = (key: FormKey) => {
    const messages = fieldErrors[key];
    if (!messages?.length) return null;
    return (
      <span className="field-error" id={`error-${key}`}>
        {messages.map((text) => <span key={text}>{text}</span>)}
      </span>
    );
  };
  const invalidProps = (key: FormKey) =>
    fieldErrors[key]?.length ? { "aria-invalid": true, "aria-describedby": `error-${key}` } : {};

  const logLevelOptions = ["debug", "info", "warn", "error"];

  const field = (
    key: "dns_listen" | "dns_upstreams" | "dns_allowed_clients" | "http_listen" | "http_allowed_hosts",
    label: string,
    placeholder: string,
    hint: string,
  ) => (
    <label className="field">
      <span>{label}</span>
      <input
        type="text"
        value={formData[key]}
        onChange={(e) => handleChange(key, e.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        {...invalidProps(key)}
      />
      {errorsFor(key)}
      <small>{hint}</small>
    </label>
  );

  return (
    <div className="settings-layout">
      <section className="panel">
        <div className="settings-card">
          <div className="settings-card-intro">
            <h3>{t("settings.preferences")}</h3>
            <p>{t("settings.theme_hint")}</p>
          </div>
          <div className="settings-preferences">
            <label>
              {t("settings.language")}
              <select value={language} onChange={(e) => setLanguage(e.target.value as Language)}>
                {SUPPORTED_LANGUAGES.map((code) => (
                  <option key={code} value={code}>
                    {languageName(code)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t("settings.theme")}
              <select value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
                {SUPPORTED_THEMES.map((code) => (
                  <option key={code} value={code}>
                    {t(`settings.theme_${code}`)}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("settings.server_configuration")}</h2>
            <p>{t("settings.edit_hint")}</p>
          </div>
        </div>
        <div className="settings-card">
          <div className="settings-card-intro">
            <h3>{t("settings.dns_section")}</h3>
          </div>
          <div className="settings-fields">
            {field("dns_listen", t("settings.dns_listen_label"), "127.0.0.1:53, [::1]:53", t("settings.dns_listen_warning"))}
            {field("dns_upstreams", t("settings.upstreams_label"), "1.1.1.1:53, 9.9.9.9:53", t("settings.upstreams_hint"))}
            {field("dns_allowed_clients", t("settings.allowed_clients_label"), "127.0.0.0/8, ::1/128", t("settings.allowed_clients_hint"))}
          </div>
        </div>
        <div className="settings-card">
          <div className="settings-card-intro">
            <h3>{t("settings.cache_section")}</h3>
          </div>
          <div className="settings-fields">
            <label className="field">
              <span>{t("settings.cache_ttl_label")}</span>
              <input
                type="number"
                min={0}
                max={604800}
                step={1}
                value={formData.cache_upstream_ttl}
                onChange={(e) => handleChange("cache_upstream_ttl", e.target.value)}
                {...invalidProps("cache_upstream_ttl")}
              />
              {errorsFor("cache_upstream_ttl")}
              <small>{t("settings.cache_ttl_hint")}</small>
            </label>
          </div>
        </div>
        <div className="settings-card">
          <div className="settings-card-intro">
            <h3>{t("settings.web_section")}</h3>
          </div>
          <div className="settings-fields">
            {field("http_listen", t("settings.web_listen_label"), "127.0.0.1:8080", t("settings.web_listen_warning"))}
            {field("http_allowed_hosts", t("settings.allowed_hosts_label"), "localhost, 127.0.0.1, ::1", t("settings.allowed_hosts_hint"))}
          </div>
        </div>
        <div className="settings-card">
          <div className="settings-card-intro">
            <h3>{t("settings.logging_section")}</h3>
          </div>
          <div className="settings-fields">
            <label className="check-field">
              <input
                type="checkbox"
                checked={formData.query_log_enabled}
                onChange={(e) => handleChange("query_log_enabled", e.target.checked)}
              />
              <span>{t("settings.enable_query_log")}</span>
            </label>
            <label className="field">
              <span>{t("settings.log_level")}</span>
              <select value={formData.log_level} onChange={(e) => handleChange("log_level", e.target.value)} {...invalidProps("log_level")}>
                {logLevelOptions.map((level) => (
                  <option key={level} value={level}>{level}</option>
                ))}
              </select>
              {errorsFor("log_level")}
            </label>
          </div>
        </div>
        <div className="save-bar">
          {message && (
            <div className={`notice ${message.type === "error" ? "error" : "success"}`} role="alert">
              {message.type === "error" ? <AlertCircle size={16} /> : <CheckCircle size={16} />}
              {message.text}
            </div>
          )}
          <button className="button" onClick={() => void handleCheck()} disabled={saving}>
            <ListChecks size={15} />
            {t("settings.check")}
          </button>
          <button className="button primary" onClick={handleSave} disabled={saving}>
            <Save size={15} />
            {saving ? t("settings.saving") : t("settings.save_configuration")}
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("settings.rate_limiting")}</h2>
            <p>{t("settings.rate_limit_hint")}</p>
          </div>
          {rateLimitStatus && (
            <p role="status" className="subtle-badge">
              {t("settings.rate_limit_rejected_total")}: {rateLimitStatus.rejected_total.toLocaleString(language)}
              {rateLimitStatus.last_rejected_at && (
                <> · {t("settings.rate_limit_last_rejected")}: {new Date(rateLimitStatus.last_rejected_at).toLocaleString(language)}</>
              )}
            </p>
          )}
        </div>
        {loadingRateLimit ? (
          <Loading>{t("settings.rate_limit_loading")}</Loading>
        ) : rateLimit ? (
          <fieldset className="rate-limit-fieldset">
            <div className="panel-body">
              <div className="rate-limit-grid">
                <label className="check-row">
                  <input
                    type="checkbox"
                    checked={rateLimit.enabled}
                    onChange={(e) => setRateLimit({ ...rateLimit, enabled: e.target.checked })}
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
            </div>
            <div className="save-bar">
              {rateLimitError && (
                <p className="notice error" role="alert">{rateLimitError}</p>
              )}
              {rateLimitSaved && (
                <p className="notice success" role="status">{t("settings.rate_limit_saved")}</p>
              )}
              <button className="button primary" onClick={() => void saveRateLimit()}>
                {t("settings.save")}
              </button>
            </div>
          </fieldset>
        ) : (
          rateLimitError && <p className="notice error" role="alert">{rateLimitError}</p>
        )}
      </section>
    </div>
  );
}
