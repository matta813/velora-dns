import { useEffect, useState } from "react";
import { Database, Eraser, Search, Zap } from "lucide-react";
import { request, type CacheEntryPage, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
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
  const { t } = useI18n();
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
    <>
      <div className="stats">
        <Stat
          label={t("cache.live_entries")}
          value={String(data.cache.entries)}
          note={`${t("cache.capacity")}: ${data.cache.capacity}`}
          icon={<Database size={17} />}
        />
        <Stat
          label={t("cache.hits")}
          value={String(data.cache.hits)}
          note={t("cache.since_startup")}
          icon={<Zap size={17} />}
        />
        <Stat
          label={t("cache.misses")}
          value={String(data.cache.misses)}
          note={t("cache.includes_uncacheable")}
          icon={<Search size={17} />}
        />
      </div>
      <section className="panel padded">
        <h2>{t("cache.memory_cache")}</h2>
        <p>
          {t("cache.description")}
        </p>
        <p>
          {t("cache.clear_description")}
        </p>
        <button
          className="button danger"
          disabled={busy || readOnly}
          onClick={() => void flush()}
        >
          <Eraser size={16} />
          {busy ? t("cache.clearing") : t("cache.clear")}
        </button>
        {message && <p role="status">{message}</p>}
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("cache.entries_title")}</h2>
            <p>{t("cache.entries_description")}</p>
          </div>
          <span className="subtle-badge">{entryPage?.total ?? data.cache.entries}</span>
        </div>
        {entryError && <p className="notice error" role="alert">{entryError}</p>}
        {!entryPage && !entryError && <p className="padded" role="status">{t("cache.entries_loading")}</p>}
        {entryPage?.total === 0 && <p className="padded">{t("cache.entries_empty")}</p>}
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
                    <td>{entry.type}</td>
                    <td>{entry.remaining_ttl} {t("cache.seconds")}</td>
                    <td>{entry.rcode}</td>
                    <td className="cache-answer">{entry.answers.length ? entry.answers.map((answer, i) => <code key={i}>{answer}</code>) : "—"}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
            <div className="cache-pagination">
              <span>{offset + 1}–{Math.min(offset + pageSize, entryPage.total)} / {entryPage.total}</span>
              <div className="form-actions">
                <button className="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - pageSize))}>{t("cache.previous")}</button>
                <button className="button" disabled={offset + pageSize >= entryPage.total} onClick={() => setOffset(offset + pageSize)}>{t("cache.next")}</button>
              </div>
            </div>
          </>
        )}
      </section>
    </>
  );
}
