import { useEffect, useState } from "react";
import { EyeOff, Filter, RefreshCw, SearchX, ShieldBan } from "lucide-react";
import { request, type QueryLogEntry } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import type { BadgeTone } from "../components/tone";
const empty = { domain: "", client: "", type: "", source: "" };
const SOURCES = ["cache", "local", "upstream", "blocked", "refused", "overload"];
const SOURCE_TONES: Record<string, BadgeTone> = {
  cache: "info",
  local: "brand",
  upstream: "neutral",
  blocked: "danger",
  refused: "warning",
  overload: "warning",
};
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
      <section className="panel" role="status">
        <EmptyState icon={<EyeOff size={22} />} title={t("querylog.unavailable_title")}>
          {t("querylog.unavailable_text")}
        </EmptyState>
      </section>
    );
  }
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">{t("querylog.up_to_100")}</span>
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
          <div className="form-grid filters">
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
                {["A", "AAAA", "CNAME", "TXT", "MX", "NS", "PTR", "SOA"].map((v) => (
                  <option key={v}>{v}</option>
                ))}
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
                {SOURCES.map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </label>
          </div>
          <div className="form-actions">
            <button className="button primary" disabled={loading}>
              <Filter size={15} />
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
              <ShieldBan size={15} />
              {t("querylog.show_blocked")}
            </button>
          </div>
        </form>
      </section>
      {error && (
        <div className="notice error" role="alert">
          {error}. {t("querylog.error_stale")}
        </div>
      )}
      {loading && (
        <section className="panel form-panel">
          <Loading>{t("querylog.loading")}</Loading>
        </section>
      )}
      {entries?.length === 0 && !error && !loading && (
        <section className="panel">
          <EmptyState icon={<SearchX size={22} />} title={t("querylog.no_matches")}>
            {t("querylog.no_matches_text")}
          </EmptyState>
        </section>
      )}
      {Boolean(entries?.length) && (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">{t("querylog.caption")}</caption>
              <thead>
                <tr>
                  <th>{t("querylog.col_timestamp")}</th>
                  <th>{t("querylog.col_client")}</th>
                  <th>{t("querylog.col_domain")}</th>
                  <th>{t("querylog.col_type")}</th>
                  <th>{t("querylog.col_source")}</th>
                  <th>{t("querylog.col_response")}</th>
                  <th className="num">{t("querylog.col_latency")}</th>
                </tr>
              </thead>
              <tbody>
                {entries?.map((e) => (
                  <tr key={e.id}>
                    <td className="cell-muted">{new Date(e.occurred_at).toLocaleString()}</td>
                    <td>
                      <code>{e.client_ip}</code>
                    </td>
                    <td className="record-value">
                      <code>{e.domain}</code>
                    </td>
                    <td>
                      <span className={`record-type type-${e.type}`}>{e.type}</span>
                    </td>
                    <td title={e.upstream || undefined}>
                      <Badge tone={SOURCE_TONES[e.source] ?? "neutral"}>{e.source}</Badge>
                    </td>
                    <td>
                      <span className={e.rcode === "NOERROR" ? "cell-muted" : "danger"}>{e.rcode}</span>
                    </td>
                    <td className="num mono">{(e.duration / 1e6).toFixed(2)} ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {entries?.length === 100 && (
            <div className="panel-footer">
              <span />
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
            </div>
          )}
        </section>
      )}
    </>
  );
}
