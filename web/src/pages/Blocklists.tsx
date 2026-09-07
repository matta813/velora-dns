import { useEffect, useState } from "react";
import { Plus, RefreshCw, ShieldBan } from "lucide-react";
import { request, type BlocklistSource } from "../api";

export function Blocklists() {
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
      setError(e instanceof Error ? e.message : "Unable to load blocklists");
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
      setError(e instanceof Error ? e.message : "Unable to add source");
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
      setError(e instanceof Error ? e.message : "Unable to update source");
    } finally {
      setBusy(null);
    }
  };
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {sources
            ? `${sources.length} external ${sources.length === 1 ? "source" : "sources"}`
            : "DNS policy sources"}
        </span>
        <div>
          <button
            className="button"
            onClick={() => void load()}
            disabled={busy !== null}
          >
            <RefreshCw size={15} />
            Reload
          </button>
          <button
            className="button primary"
            onClick={() => setAdding(true)}
            disabled={busy !== null}
          >
            <Plus size={15} />
            Add source
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
            aria-label="Add blocklist source"
            onSubmit={(e) => {
              e.preventDefault();
              void add();
            }}
          >
            <h3>Add external blocklist</h3>
            <p>
              Only public HTTP(S) sources are accepted. Failed refreshes keep
              prior DNS rules active.
            </p>
            <fieldset disabled={busy !== null}>
              <label>
                Name
                <input
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Community hosts"
                />
              </label>
              <label>
                Source URL
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
                  {busy === "new" ? "Adding…" : "Add source"}
                </button>
                <button
                  type="button"
                  className="button"
                  onClick={() => setAdding(false)}
                >
                  Cancel
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {sources === null ? (
        <section className="panel padded" role="status">
          Loading blocklist sources…
        </section>
      ) : sources.length === 0 ? (
        <section className="panel zone-empty large">
          <ShieldBan size={32} />
          <h2>No external blocklists</h2>
          <p>
            Add a trusted public hosts or domain list to apply DNS blocking.
          </p>
        </section>
      ) : (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">External blocklist sources</caption>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Source</th>
                  <th>Status</th>
                  <th>Last update</th>
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
                        <span className="notice error">Update failed</span>
                      ) : (
                        <span className="record-type">
                          {source.enabled ? "ACTIVE" : "DISABLED"}
                        </span>
                      )}
                    </td>
                    <td>
                      {source.last_updated_at
                        ? new Date(source.last_updated_at).toLocaleString()
                        : "Never"}
                    </td>
                    <td>
                      <button
                        className="button"
                        disabled={busy !== null}
                        onClick={() => void refresh(source.id)}
                      >
                        <RefreshCw size={14} />
                        {busy === source.id ? "Updating…" : "Update"}
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
