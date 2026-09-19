import type { Snapshot } from "../api";
import { loadRateLimitSettings, saveRateLimitSettings, type RateLimitSettings } from "../api";
import { useEffect, useState } from "react";
import { SUPPORTED_LANGUAGES, languageName, type Language } from "../i18n";
import { useI18n } from "../i18n-context";

export function Settings({ data }: { data: Snapshot }) {
  const { t, language, setLanguage } = useI18n();
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
  async function save() {
    if (!rateLimit) return;
    setRateLimitError("");
    setRateLimitSaved(false);
    try {
      const saved = await saveRateLimitSettings(rateLimit);
      setRateLimit(saved);
      setRateLimitSaved(true);
    } catch (e) {
      setRateLimitError(e instanceof Error ? e.message : t("settings.rate_limit_save_failed"));
    }
  }
  const rows: [string, string][] = [
    [t("settings.dns_listeners"), data.status.dns_listen.join(", ")],
    [t("settings.allowed_clients"), data.config.dns.allowed_clients.join(", ")],
    [t("settings.upstreams"), data.config.dns.upstreams.join(", ")],
    [t("settings.attempt_timeout"), `${data.config.dns.timeout / 1e9} ${t("settings.seconds")}`],
    [t("settings.retries"), String(data.config.dns.retries)],
    [t("settings.max_concurrent"), String(data.config.dns.max_concurrent)],
    [t("settings.cache_capacity"), String(data.cache.capacity)],
    [t("settings.log_level"), data.config.log_level],
    [t("settings.version"), data.status.version.version],
  ];
  return (
    <>
      <section className="panel padded">
        <h2>{t("settings.server_configuration")}</h2>
        <p>
          {t("settings.read_only_hint")}
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
        </div>
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
                  value={rateLimit.global_qps}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, global_qps: Number(e.target.value) })
                  }
                />
              </label>
              <label>
                {t("settings.rate_limit_client_qps")}
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={rateLimit.client_qps}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, client_qps: Number(e.target.value) })
                  }
                />
              </label>
              <label>
                {t("settings.rate_limit_burst")}
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={rateLimit.rate_limit_burst}
                  onChange={(e) =>
                    setRateLimit({ ...rateLimit, rate_limit_burst: Number(e.target.value) })
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
              <button className="button primary" onClick={() => void save()}>
                {t("settings.save")}
              </button>
            </div>
          </fieldset>
        ) : null}
      </section>
      <div className="notice">
        {t("settings.security_notice")}
      </div>
    </>
  );
}