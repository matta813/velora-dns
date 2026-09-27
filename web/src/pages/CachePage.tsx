import { useEffect, useState, type FormEvent } from "react";
import { Database, Eraser, Trash2 } from "lucide-react";
import { request, type CacheEntryPage, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";

const RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "NS", "PTR", "SOA", "TXT"];
const PAGE_SIZE = 100;

interface InvalidateResult {
  removed: number;
}

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
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [entryPage, setEntryPage] = useState<CacheEntryPage | null>(null);
  const [entryError, setEntryError] = useState("");
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const [filter, setFilter] = useState({ domain: "", type: "" });
  const [domain, setDomain] = useState("");
  const [includeSubdomains, setIncludeSubdomains] = useState(true);

  // Debounce typing so the cache is not scanned on every keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setOffset(0);
      setFilter({ domain: search.trim(), type: typeFilter });
    }, 250);
    return () => window.clearTimeout(timer);
  }, [search, typeFilter]);

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
    if (filter.domain) params.set("domain", filter.domain);
    if (filter.type) params.set("type", filter.type);
    void request<CacheEntryPage>(`/api/v1/cache/entries?${params}`, controller.signal)
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
  }, [offset, revision, filter, data.checked, t]);

  const afterChange = () => {
    setRevision((value) => value + 1);
    refresh();
  };

  async function flush() {
    if (!window.confirm(t("cache.flush_confirm"))) return;
    setBusy(true);
    setMessage(null);
    try {
      await request("/api/v1/cache", undefined, "DELETE");
      setMessage({ tone: "success", text: t("cache.cleared") });
      setOffset(0);
      afterChange();
    } catch (e) {
      setMessage({ tone: "error", text: e instanceof Error ? e.message : t("cache.flush_failed") });
    } finally {
      setBusy(false);
    }
  }

  async function invalidate(name: string, type: string, subdomains: boolean) {
    setBusy(true);
    setMessage(null);
    try {
      const result = await request<InvalidateResult>("/api/v1/cache/invalidate", undefined, "POST", {
        body: { name, type, include_subdomains: subdomains },
      });
      setMessage({ tone: "success", text: `${t("cache.removed_entries")}: ${result.removed}` });
      afterChange();
      return true;
    } catch (e) {
      setMessage({ tone: "error", text: e instanceof Error ? e.message : t("cache.invalidate_failed") });
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function submitDomain(event: FormEvent) {
    event.preventDefault();
    if (await invalidate(domain.trim(), "", includeSubdomains)) setDomain("");
  }

  const filtering = Boolean(filter.domain || filter.type);

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
      {message && (
        <div className={`notice ${message.tone}`} role={message.tone === "error" ? "alert" : "status"}>
          {message.text}
        </div>
      )}
      <div className="grid-2">
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("cache.memory_cache")}</h2>
              <p>{t("cache.description")}</p>
            </div>
          </div>
          <div className="panel-body stack-sm">
            <p className="field-hint">{t("cache.clear_description")}</p>
            <div>
              <button className="button outline-danger" disabled={busy || readOnly} onClick={() => void flush()}>
                <Eraser size={15} />
                {busy ? t("cache.clearing") : t("cache.clear")}
              </button>
            </div>
          </div>
        </section>
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("cache.invalidate_title")}</h2>
              <p>{t("cache.invalidate_text")}</p>
            </div>
          </div>
          <form className="panel-body stack-sm" onSubmit={(event) => void submitDomain(event)}>
            <fieldset disabled={busy || readOnly} className="stack-sm">
              <label className="field">
                {t("cache.domain")}
                <input
                  value={domain}
                  onChange={(event) => setDomain(event.target.value)}
                  placeholder={t("cache.search_placeholder")}
                  maxLength={253}
                  autoComplete="off"
                  required
                />
              </label>
              <label className="check-field">
                <input
                  type="checkbox"
                  checked={includeSubdomains}
                  onChange={(event) => setIncludeSubdomains(event.target.checked)}
                />
                {t("cache.include_subdomains")}
              </label>
              <div>
                <button className="button" type="submit">
                  {t("cache.invalidate")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      </div>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("cache.entries_title")}</h2>
            <p>{t("cache.entries_description")}</p>
          </div>
          <span className="subtle-badge">{(entryPage?.total ?? data.cache.entries).toLocaleString(language)}</span>
        </div>
        <div className="zone-form" role="search">
          <div className="form-grid cache-filters">
            <label>
              {t("cache.search_label")}
              <input
                type="search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("cache.search_placeholder")}
                maxLength={253}
                autoComplete="off"
              />
            </label>
            <label>
              {t("cache.entry_type")}
              <select value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}>
                <option value="">{t("cache.all_types")}</option>
                {RECORD_TYPES.map((type) => (
                  <option key={type}>{type}</option>
                ))}
              </select>
            </label>
          </div>
        </div>
        {entryError && <p className="notice error" role="alert">{entryError}</p>}
        {!entryPage && !entryError && <Loading>{t("cache.entries_loading")}</Loading>}
        {entryPage?.total === 0 && (
          <EmptyState
            compact
            icon={filtering ? undefined : <Database size={22} />}
            title={filtering ? t("cache.no_matches") : t("cache.entries_empty")}
          />
        )}
        {entryPage && entryPage.total > 0 && (
          <>
            <p className="cache-scroll-hint">{t("cache.entries_scroll_hint")}</p>
            <div className="table-wrap">
              <table className="cache-entries-table">
                <caption className="sr-only">{t("cache.entries_title")}</caption>
                <thead>
                  <tr>
                    <th>{t("cache.entry_name")}</th>
                    <th>{t("cache.entry_type")}</th>
                    <th>{t("cache.entry_ttl")}</th>
                    <th>{t("cache.entry_status")}</th>
                    <th>{t("cache.entry_answer")}</th>
                    <th>
                      <span className="sr-only">{t("zones.col_actions")}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {entryPage.entries.map((entry, index) => (
                    <tr key={`${entry.name}-${entry.type}-${index}`}>
                      <td><code>{entry.name}</code></td>
                      <td><span className="record-type">{entry.type}</span></td>
                      <td className="mono cell-muted">{entry.remaining_ttl} {t("cache.seconds")}</td>
                      <td><span className={entry.rcode === "NOERROR" ? "cell-muted" : "danger"}>{entry.rcode}</span></td>
                      <td className="cache-answer">
                        {entry.answers.length
                          ? entry.answers.map((answer, i) => <code key={i}>{answer}</code>)
                          : <span className="cell-muted">—</span>}
                      </td>
                      <td>
                        <div className="table-actions">
                          <button
                            className="icon-button danger-icon"
                            disabled={busy || readOnly}
                            onClick={() => void invalidate(entry.name, entry.type, false)}
                            aria-label={`${t("cache.remove_entry")}: ${entry.name} ${entry.type}`}
                            title={t("cache.remove_entry")}
                          >
                            <Trash2 size={15} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="cache-pagination">
              <span>{offset + 1}–{Math.min(offset + PAGE_SIZE, entryPage.total)} / {entryPage.total}</span>
              <div className="form-actions">
                <button className="button small" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>{t("cache.previous")}</button>
                <button className="button small" disabled={offset + PAGE_SIZE >= entryPage.total} onClick={() => setOffset(offset + PAGE_SIZE)}>{t("cache.next")}</button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  );
}
