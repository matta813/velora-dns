import { Download, CheckCircle, XCircle, Clock, AlertTriangle } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import {
  loadUpdateStatus,
  loadUpdateHistory,
  requestUpdate,
  type UpdateStatus,
  type UpdateEntry,
} from "../api";
import { useI18n } from "../i18n-context";

interface Props {
  readOnly: boolean;
}

export function UpdateCenter({ readOnly }: Props) {
  const { t } = useI18n();
  const stateLabel = (state: string) => {
    const key = `updates.state_${state}`;
    const translated = t(key);
    return translated === key ? state : translated;
  };
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [history, setHistory] = useState<UpdateEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [requesting, setRequesting] = useState(false);
  const [requestMessage, setRequestMessage] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  const refresh = useCallback(() => setRevision((v) => v + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const [s, h] = await Promise.all([
          loadUpdateStatus(controller.signal),
          loadUpdateHistory(controller.signal),
        ]);
        if (controller.signal.aborted) return;
        setStatus(s);
        setHistory(h);
        setError(null);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : t("updates.load_failed"));
      } finally {
        setLoading(false);
        if (!controller.signal.aborted) timer = setTimeout(poll, 10000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [revision, t]);

  const handleRequestUpdate = async () => {
    if (!confirm(t("updates.confirm"))) return;
    setRequesting(true);
    setRequestMessage(null);
    try {
      const result = await requestUpdate();
      setRequestMessage(result.message);
      refresh();
    } catch (e) {
      setRequestMessage(e instanceof Error ? e.message : t("updates.request_failed"));
    } finally {
      setRequesting(false);
    }
  };

  if (loading) {
    return <div className="panel padded">{t("updates.loading")}</div>;
  }

  return (
    <div>
      <div className="panel padded">
        <h2>{t("updates.current_status")}</h2>
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
                  ? `${t("updates.updating")} ${stateLabel(status.state)}`
                  : `${t("updates.installed")} ${status.installed}`}
              </strong>
            </div>
            {status.updating && status.from_version && status.to_version && (
              <div>
                {status.from_version} &rarr; {status.to_version}
              </div>
            )}
            {status.last_completed && (
              <div style={{ fontSize: "0.85rem", opacity: 0.7 }}>
                {t("updates.last_completed")} {new Date(status.last_completed).toLocaleString()}
              </div>
            )}
          </div>
        )}
        <div style={{ marginTop: "1rem" }}>
          {readOnly ? (
            <div className="notice" role="status">
              {t("updates.viewer_readonly")}
            </div>
          ) : (
            <button
              className="button"
              onClick={handleRequestUpdate}
              disabled={requesting || status?.updating}
            >
              <Download size={15} />
              {requesting ? t("updates.requesting") : status?.updating ? t("updates.in_progress") : t("updates.request")}
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
        <h2>{t("updates.history")}</h2>
        {history.length === 0 ? (
          <p style={{ opacity: 0.6 }}>{t("updates.no_history")}</p>
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_from")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_to")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_channel")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_state")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_mode")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_started")}</th>
                <th style={{ textAlign: "left", padding: "0.5rem" }}>{t("updates.col_duration")}</th>
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
                        entry.channel === "stable"
                          ? "badge-success"
                          : entry.channel === "beta"
                            ? "badge-info"
                            : entry.channel === "alpha"
                              ? "badge-warning"
                              : ""
                      }`}
                    >
                      {entry.channel || "stable"}
                    </span>
                  </td>
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
                      {stateLabel(entry.state)}
                    </span>
                    {entry.rollback_used && t("updates.rolled_back")}
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
