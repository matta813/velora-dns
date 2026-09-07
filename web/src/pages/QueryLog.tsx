import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { request, type QueryLogEntry } from "../api";
const empty = { domain: "", client: "", type: "", source: "" };
export function QueryLog({ enabled }: { enabled: boolean }) {
  const [entries, setEntries] = useState<QueryLogEntry[] | null>(null);
  const [draft, setDraft] = useState(empty);
  const [filter, setFilter] = useState({ ...empty, before: "", revision: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
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
          setError(e instanceof Error ? e.message : "Unable to load query log");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [filter]);
  function apply(before = "") {
    setLoading(true);
    setFilter({ ...draft, before, revision: filter.revision + 1 });
  }
  return (
    <>
      {!enabled && (
        <div className="notice">
          Query logging is disabled. Existing history remains visible until its
          retention expires.
        </div>
      )}
      <div className="zones-toolbar">
        <span className="zone-count">
          Up to 100 retained responses per page
        </span>
        <button className="button" disabled={loading} onClick={() => apply()}>
          <RefreshCw size={15} />
          Refresh queries
        </button>
      </div>
      <section className="panel form-panel">
        <form
          className="zone-form"
          aria-label="Query filters"
          onSubmit={(e) => {
            e.preventDefault();
            apply();
          }}
        >
          <div className="form-grid">
            <label>
              Domain
              <input
                maxLength={253}
                value={draft.domain}
                onChange={(e) => setDraft({ ...draft, domain: e.target.value })}
                placeholder="Contains example.com"
              />
            </label>
            <label>
              Client IP
              <input
                value={draft.client}
                onChange={(e) => setDraft({ ...draft, client: e.target.value })}
                placeholder="Exact IP address"
              />
            </label>
            <label>
              Query type
              <select
                aria-label="Query type"
                value={draft.type}
                onChange={(e) => setDraft({ ...draft, type: e.target.value })}
              >
                <option value="">All types</option>
                {["A", "AAAA", "CNAME", "TXT", "MX", "NS", "PTR", "SOA"].map(
                  (v) => (
                    <option key={v}>{v}</option>
                  ),
                )}
              </select>
            </label>
            <label>
              Source
              <select
                aria-label="Source"
                value={draft.source}
                onChange={(e) => setDraft({ ...draft, source: e.target.value })}
              >
                <option value="">All sources</option>
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
            Apply filters
          </button>
        </form>
      </section>
      {error && (
        <div className="notice error" role="alert">
          {error}. Previous results may be out of date.
        </div>
      )}
      {loading && <p role="status">Loading query log…</p>}
      {entries?.length === 0 && !error && !loading && (
        <section className="panel zone-empty large">
          <h2>No matching queries</h2>
          <p>
            Try different filters or generate DNS traffic with logging enabled.
          </p>
        </section>
      )}
      {Boolean(entries?.length) && (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">DNS query log</caption>
              <thead>
                <tr>
                  {[
                    "Timestamp",
                    "Client",
                    "Domain",
                    "Type",
                    "Source",
                    "Response",
                    "Latency",
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
          Older queries
        </button>
      )}
    </>
  );
}
