import { useEffect, useState } from "react";
import { Plus, RefreshCw, ShieldBan } from "lucide-react";
import { request, type BlocklistSource } from "../api";
import { useI18n } from "../i18n-context";

export function Blocklists({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
  const [sources, setSources] = useState<BlocklistSource[] | null>(null);
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState<number | "new" | null>(null);
  const [error, setError] = useState("");
  const load = async () => {
    setError("");
    try {
      setSources(await request<BlocklistSource[]>("/api/v1/blocklists"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.load_failed"));
    }
  };
  useEffect(() => {
    void Promise.resolve().then(load);
  }, []);
  const add = async () => {
    setBusy("new");
    setError("");
    try {
      await request("/api/v1/blocklists", undefined, "POST", {
        body: { name, url },
      });
      setName("");
      setURL("");
      setAdding(false);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.add_failed"));
    } finally {
      setBusy(null);
    }
  };
  const refresh = async (id: number) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}/update`, undefined, "POST");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.update_failed_msg"));
    } finally {
      setBusy(null);
    }
  };
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {sources
            ? `${sources.length} ${sources.length === 1 ? t("blocklists.count_one") : t("blocklists.count")}`
            : t("blocklists.sources_label")}
        </span>
        <div>
          <button
            className="button"
            onClick={() => void load()}
            disabled={busy !== null}
          >
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button
            className="button primary"
            onClick={() => setAdding(true)}
            disabled={busy !== null || readOnly}
          >
            <Plus size={15} />
            {t("blocklists.add_source")}
          </button>
        </div>
      </div>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      {adding && (
        <section className="panel form-panel">
          <form
            className="zone-form"
            aria-label={t("blocklists.add_aria")}
            onSubmit={(e) => {
              e.preventDefault();
              void add();
            }}
          >
            <h3>{t("blocklists.add_title")}</h3>
            <p>
              {t("blocklists.add_text")}
            </p>
            <fieldset disabled={busy !== null}>
              <label>
                Name
                <input
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("blocklists.community_hosts")}
                />
              </label>
              <label>
                {t("blocklists.source_url")}
                <input
                  required
                  type="url"
                  value={url}
                  onChange={(e) => setURL(e.target.value)}
                  placeholder="https://example.org/hosts.txt"
                />
              </label>
              <div className="form-actions">
                <button className="button primary">
                  {busy === "new" ? t("blocklists.adding") : t("blocklists.add_source")}
                </button>
                <button
                  type="button"
                  className="button"
                  onClick={() => setAdding(false)}
                >
                  {t("blocklists.cancel")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {sources === null ? (
        <section className="panel padded" role="status">
          {t("blocklists.loading")}
        </section>
      ) : sources.length === 0 ? (
        <section className="panel zone-empty large">
          <ShieldBan size={32} />
          <h2>{t("blocklists.empty_title")}</h2>
          <p>
            {t("blocklists.empty_text")}
          </p>
        </section>
      ) : (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">{t("blocklists.caption")}</caption>
              <thead>
                <tr>
                  <th>{t("blocklists.name")}</th>
                  <th>{t("blocklists.col_source")}</th>
                  <th>{t("blocklists.col_status")}</th>
                  <th>{t("blocklists.col_last_update")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {sources.map((source) => (
                  <tr key={source.id}>
                    <td>
                      <strong>{source.name}</strong>
                    </td>
                    <td className="record-value">
                      <code>{source.url}</code>
                    </td>
                    <td>
                      {source.last_error ? (
                        <span className="notice error">{t("blocklists.update_failed")}</span>
                      ) : (
                        <span className="record-type">
                          {source.enabled ? t("blocklists.active") : t("blocklists.disabled")}
                        </span>
                      )}
                    </td>
                    <td>
                      {source.last_updated_at
                        ? new Date(source.last_updated_at).toLocaleString()
                        : t("blocklists.never")}
                    </td>
                    <td>
                      <button
                        className="button"
                        disabled={busy !== null || readOnly}
                        onClick={() => void refresh(source.id)}
                      >
                        <RefreshCw size={14} />
                        {busy === source.id ? t("blocklists.updating") : t("blocklists.update")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
