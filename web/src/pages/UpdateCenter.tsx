import { Download, CheckCircle, XCircle, Clock, AlertTriangle } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import {
  loadUpdateStatus,
  loadUpdateHistory,
  requestUpdate,
  type UpdateStatus,
  type UpdateEntry,
} from "../api";

interface Props {
  readOnly: boolean;
}

export function UpdateCenter({ readOnly }: Props) {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [history, setHistory] = useState<UpdateEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [requesting, setRequesting] = useState(false);
  const [requestMessage, setRequestMessage] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [s, h] = await Promise.all([loadUpdateStatus(), loadUpdateHistory()]);
      setStatus(s);
      setHistory(h);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load update status");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 10000);
    return () => clearInterval(interval);
  }, [refresh]);

  const handleRequestUpdate = async () => {
    if (!confirm("Are you sure you want to request an update?")) return;
    setRequesting(true);
    setRequestMessage(null);
    try {
      const result = await requestUpdate();
      setRequestMessage(result.message);
      await refresh();
    } catch (e) {
      setRequestMessage(e instanceof Error ? e.message : "Failed to request update");
    } finally {
      setRequesting(false);
    }
  };

  if (loading) {
    return <div className="panel padded">Loading update status...</div>;
  }

  return (
    <div>
      <div className="panel padded">
        <h2>Current Status</h2>
        {error && (
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
        {status && (
          <div style={{ display: "grid", gap: "1rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              {status.updating ? (
                <Clock size={18} className="spin" />
              ) : status.state === "completed" ? (
                <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />
              ) : status.state === "failed" || status.state === "rolled_back" ? (
                <XCircle size={18} style={{ color: "var(--error, #ef4444)" }} />
              ) : (
                <Download size={18} />
              )}
              <strong>
                {status.updating
                  ? `Updating: ${status.state}`
                  : `Installed: ${status.installed}`}
              </strong>
            </div>
            {status.updating && status.from_version && status.to_version && (
              <div>
                {status.from_version} &rarr; {status.to_version}
              </div>
            )}
            {status.last_completed && (
              <div style={{ fontSize: "0.85rem", opacity: 0.7 }}>
                Last completed: {new Date(status.last_completed).toLocaleString()}
              </div>
            )}
          </div>
        )}
        <div style={{ marginTop: "1rem" }}>
          {readOnly ? (
            <div className="notice" role="status">
              You are signed in as a viewer. Update requests are read-only.
            </div>
          ) : (
            <button
              className="button"
              onClick={handleRequestUpdate}
              disabled={requesting || status?.updating}
            >
              <Download size={15} />
              {requesting ? "Requesting..." : status?.updating ? "Update in progress..." : "Request Update"}
            </button>
          )}
          {requestMessage && (
            <div
              className="notice"
              style={{ marginTop: "0.5rem" }}
              role="status"
            >
              <AlertTriangle size={15} /> {requestMessage}
            </div>
          )}
        </div>
      </div>

      <div className="panel padded" style={{ marginTop: "1rem" }}>
        <h2>Update History</h2>
        {history.length === 0 ? (
          <p style={{ opacity: 0.6 }}>No updates recorded yet.</p>
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>From</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>To</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>State</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>Mode</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>Started</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>Duration</th>
              </tr>
            </thead>
            <tbody>
              {history.map((entry) => (
                <tr key={entry.id}>
                  <td style={{ padding: "0.5rem" }}>{entry.from_version}</td>
                  <td style={{ padding: "0.5rem" }}>{entry.to_version}</td>
                  <td style={{ padding: "0.5rem" }}>
                    <span
                      className={`badge ${
                        entry.state === "completed"
                          ? "badge-success"
                          : entry.state === "failed"
                            ? "badge-error"
                            : entry.state === "rolled_back"
                              ? "badge-warning"
                              : ""
                      }`}
                    >
                      {entry.state}
                    </span>
                    {entry.rollback_used && " (rolled back)"}
                  </td>
                  <td style={{ padding: "0.5rem" }}>{entry.deployment_mode}</td>
                  <td style={{ padding: "0.5rem" }}>
                    {new Date(entry.started_at).toLocaleString()}
                  </td>
                  <td style={{ padding: "0.5rem" }}>
                    {entry.completed_at
                      ? `${(
                          (new Date(entry.completed_at).getTime() -
                            new Date(entry.started_at).getTime()) /
                          1000
                        ).toFixed(1)}s`
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
