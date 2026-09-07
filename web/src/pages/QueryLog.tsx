import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { request, type QueryLogEntry } from "../api";
export function QueryLog() {
  const [entries, setEntries] = useState<QueryLogEntry[] | null>(null);
  const [domain, setDomain] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      setError("");
      setEntries(
        await request<QueryLogEntry[]>(
          `/api/v1/queries?domain=${encodeURIComponent(domain)}&limit=100`,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load query log");
    }
  }, [domain]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">Recent DNS responses</span>
        <button className="button" onClick={() => void load()}>
          <RefreshCw size={15} />
          Refresh
        </button>
      </div>
      <section className="panel form-panel">
        <div className="zone-form">
          <label>
            Domain filter
            <input
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.com"
            />
          </label>
          <button className="button primary" onClick={() => void load()}>
            Apply filter
          </button>
        </div>
      </section>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      {entries === null ? (
        <section className="panel padded">Loading query log…</section>
      ) : entries.length === 0 ? (
        <section className="panel zone-empty large">
          <h2>No retained queries</h2>
          <p>Query logging is disabled by default.</p>
        </section>
      ) : (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <thead>
                <tr>
                  <th>Timestamp</th>
                  <th>Client</th>
                  <th>Domain</th>
                  <th>Type</th>
                  <th>Source</th>
                  <th>Response</th>
                  <th>Latency</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e, i) => (
                  <tr key={`${e.occurred_at}-${i}`}>
                    <td>{new Date(e.occurred_at).toLocaleString()}</td>
                    <td>{e.client_ip}</td>
                    <td>{e.domain}</td>
                    <td>{e.type}</td>
                    <td>{e.source}</td>
                    <td>{e.rcode}</td>
                    <td>{Math.round(e.duration / 1e6)}ms</td>
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
