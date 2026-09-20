import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { request, type QueryLogEntry } from "../api";
import { useI18n } from "../i18n-context";
const empty = { domain: "", client: "", type: "", source: "" };
export function QueryLog({ enabled }: { enabled: boolean }) {
  const { t } = useI18n();
  const [entries, setEntries] = useState<QueryLogEntry[] | null>(null);
  const [draft, setDraft] = useState(empty);
  const [filter, setFilter] = useState({ ...empty, before: "", revision: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!enabled) {
      return;
    }
    const controller = new AbortController();
    const params = new URLSearchParams({
      domain: filter.domain,
      client: filter.client,
      type: filter.type,
      source: filter.source,
      before: filter.before,
      limit: "100",
    });
    void request<QueryLogEntry[]>(
      `/api/v1/queries?${params}`,
      controller.signal,
    )
      .then((data) => {
        if (!controller.signal.aborted) {
          setEntries(data);
          setError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : t("querylog.load_failed"));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [enabled, filter, t]);
  function apply(before = "") {
    setLoading(true);
    setFilter({ ...draft, before, revision: filter.revision + 1 });
  }
  if (!enabled) {
    return (
      <section className="panel zone-empty large" role="status">
        <h2>{t("querylog.unavailable_title")}</h2>
        <p>{t("querylog.unavailable_text")}</p>
      </section>
    );
  }
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {t("querylog.up_to_100")}
        </span>
        <button className="button" disabled={loading} onClick={() => apply()}>
          <RefreshCw size={15} />
          {t("querylog.refresh")}
        </button>
      </div>
      <section className="panel form-panel">
        <form
          className="zone-form"
          aria-label={t("querylog.filters_aria")}
          onSubmit={(e) => {
            e.preventDefault();
            apply();
          }}
        >
          <div className="form-grid">
            <label>
              {t("querylog.domain")}
              <input
                maxLength={253}
                value={draft.domain}
                onChange={(e) => setDraft({ ...draft, domain: e.target.value })}
                placeholder={t("querylog.contains_example")}
              />
            </label>
            <label>
              {t("querylog.client_ip")}
              <input
                value={draft.client}
                onChange={(e) => setDraft({ ...draft, client: e.target.value })}
                placeholder={t("querylog.exact_ip")}
              />
            </label>
            <label>
              {t("querylog.query_type")}
              <select
                aria-label={t("querylog.query_type")}
                value={draft.type}
                onChange={(e) => setDraft({ ...draft, type: e.target.value })}
              >
                <option value="">{t("querylog.all_types")}</option>
                {["A", "AAAA", "CNAME", "TXT", "MX", "NS", "PTR", "SOA"].map(
                  (v) => (
                    <option key={v}>{v}</option>
                  ),
                )}
              </select>
            </label>
            <label>
              {t("querylog.source")}
              <select
                aria-label={t("querylog.source")}
                value={draft.source}
                onChange={(e) => setDraft({ ...draft, source: e.target.value })}
              >
                <option value="">{t("querylog.all_sources")}</option>
                {[
                  "cache",
                  "local",
                  "upstream",
                  "blocked",
                  "refused",
                  "overload",
                ].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </label>
          </div>
          <button className="button primary" disabled={loading}>
            {t("querylog.apply_filters")}
          </button>
          <button
            className="button"
            type="button"
            disabled={loading}
            onClick={() => {
              const blocked = { ...empty, source: "blocked" };
              setDraft(blocked);
              setLoading(true);
              setFilter({ ...blocked, before: "", revision: filter.revision + 1 });
            }}
          >
            {t("querylog.show_blocked")}
          </button>
        </form>
      </section>
      {error && (
        <div className="notice error" role="alert">
          {error}. {t("querylog.error_stale")}
        </div>
      )}
      {loading && <p role="status">{t("querylog.loading")}</p>}
      {entries?.length === 0 && !error && !loading && (
        <section className="panel zone-empty large">
          <h2>{t("querylog.no_matches")}</h2>
          <p>
            {t("querylog.no_matches_text")}
          </p>
        </section>
      )}
      {Boolean(entries?.length) && (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">{t("querylog.caption")}</caption>
              <thead>
                <tr>
                  {[
                    t("querylog.col_timestamp"),
                    t("querylog.col_client"),
                    t("querylog.col_domain"),
                    t("querylog.col_type"),
                    t("querylog.col_source"),
                    t("querylog.col_response"),
                    t("querylog.col_latency"),
                  ].map((v) => (
                    <th key={v}>{v}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {entries?.map((e) => (
                  <tr key={e.id}>
                    <td>{new Date(e.occurred_at).toLocaleString()}</td>
                    <td>
                      <code>{e.client_ip}</code>
                    </td>
                    <td className="record-value">
                      <code>{e.domain}</code>
                    </td>
                    <td>{e.type}</td>
                    <td title={e.upstream || undefined}>{e.source}</td>
                    <td>{e.rcode}</td>
                    <td>{(e.duration / 1e6).toFixed(2)} ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {entries?.length === 100 && (
        <button
          className="button"
          disabled={loading}
          onClick={() => {
            setLoading(true);
            setFilter({
              ...filter,
              before: String(entries[entries.length - 1].id),
              revision: filter.revision + 1,
            });
          }}
        >
          {t("querylog.older")}
        </button>
      )}
    </>
  );
}
