import { useEffect, useState } from "react";
import { Database, Eraser } from "lucide-react";
import { request, type CacheEntryPage, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";
export function CachePage({
  data,
  refresh,
  readOnly = false,
}: {
  data: Snapshot;
  refresh: () => void;
  readOnly?: boolean;
}) {
  const { t, language } = useI18n();
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [entryPage, setEntryPage] = useState<CacheEntryPage | null>(null);
  const [entryError, setEntryError] = useState("");
  const pageSize = 100;
  useEffect(() => {
    const controller = new AbortController();
    void request<CacheEntryPage>(`/api/v1/cache/entries?limit=${pageSize}&offset=${offset}`, controller.signal)
      .then((page) => {
        if (!controller.signal.aborted) {
          setEntryPage(page);
          setEntryError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setEntryError(e instanceof Error ? e.message : t("cache.entries_load_failed"));
      });
    return () => controller.abort();
  }, [offset, revision, data.checked, t]);
  async function flush() {
    setBusy(true);
    setMessage("");
    try {
      await request("/api/v1/cache", undefined, "DELETE");
      setMessage(t("cache.cleared"));
      setOffset(0);
      setRevision((value) => value + 1);
      refresh();
    } catch (e) {
      setMessage(e instanceof Error ? e.message : t("cache.flush_failed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="stack">
      <div className="stats">
        <Stat
          label={t("cache.live_entries")}
          value={data.cache.entries.toLocaleString(language)}
          note={`${t("cache.capacity")}: ${data.cache.capacity.toLocaleString(language)}`}
          progress={data.cache.entries / Math.max(1, data.cache.capacity)}
          tone="info"
        />
        <Stat
          label={t("cache.hits")}
          value={data.cache.hits.toLocaleString(language)}
          note={t("cache.since_startup")}
          tone="success"
        />
        <Stat
          label={t("cache.misses")}
          value={data.cache.misses.toLocaleString(language)}
          note={t("cache.includes_uncacheable")}
          tone="warning"
        />
      </div>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("cache.memory_cache")}</h2>
            <p>{t("cache.description")}</p>
          </div>
          <button className="button danger" disabled={busy || readOnly} onClick={() => void flush()}>
            <Eraser size={15} />
            {busy ? t("cache.clearing") : t("cache.clear")}
          </button>
        </div>
        <div className="panel-body">
          <p className="field-hint">{t("cache.clear_description")}</p>
          {message && <p className="notice" role="status">{message}</p>}
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("cache.entries_title")}</h2>
            <p>{t("cache.entries_description")}</p>
          </div>
          <span className="subtle-badge">{(entryPage?.total ?? data.cache.entries).toLocaleString(language)}</span>
        </div>
        {entryError && <p className="notice error" role="alert">{entryError}</p>}
        {!entryPage && !entryError && <Loading>{t("cache.entries_loading")}</Loading>}
        {entryPage?.total === 0 && <EmptyState compact icon={<Database size={22} />} title={t("cache.entries_empty")} />}
        {entryPage && entryPage.total > 0 && (
          <>
            <p className="cache-scroll-hint">{t("cache.entries_scroll_hint")}</p>
            <div className="table-wrap">
              <table className="cache-entries-table">
                <caption className="sr-only">{t("cache.entries_title")}</caption>
                <thead><tr>
                  <th>{t("cache.entry_name")}</th>
                  <th>{t("cache.entry_type")}</th>
                  <th>{t("cache.entry_ttl")}</th>
                  <th>{t("cache.entry_status")}</th>
                  <th>{t("cache.entry_answer")}</th>
                </tr></thead>
                <tbody>{entryPage.entries.map((entry, index) => (
                  <tr key={`${entry.name}-${entry.type}-${index}`}>
                    <td><code>{entry.name}</code></td>
                    <td><span className={`record-type type-${entry.type}`}>{entry.type}</span></td>
                    <td className="mono cell-muted">{entry.remaining_ttl} {t("cache.seconds")}</td>
                    <td><span className={entry.rcode === "NOERROR" ? "cell-muted" : "danger"}>{entry.rcode}</span></td>
                    <td className="cache-answer">{entry.answers.length ? entry.answers.map((answer, i) => <code key={i}>{answer}</code>) : <span className="cell-muted">—</span>}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
            <div className="cache-pagination">
              <span>{offset + 1}–{Math.min(offset + pageSize, entryPage.total)} / {entryPage.total}</span>
              <div className="form-actions">
                <button className="button small" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - pageSize))}>{t("cache.previous")}</button>
                <button className="button small" disabled={offset + pageSize >= entryPage.total} onClick={() => setOffset(offset + pageSize)}>{t("cache.next")}</button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  );
}
