import type { Snapshot } from "../api";
import { SUPPORTED_LANGUAGES, languageName, type Language } from "../i18n";
import { useI18n } from "../i18n-context";

export function Settings({ data }: { data: Snapshot }) {
  const { t, language, setLanguage } = useI18n();
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
      <div className="notice">
        {t("settings.security_notice")}
      </div>
    </section>
  );
}