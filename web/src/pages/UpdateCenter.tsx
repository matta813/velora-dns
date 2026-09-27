import { Download, CheckCircle, XCircle, Clock, AlertTriangle, RefreshCw, History } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import {
  loadUpdateStatus,
  loadUpdateHistory,
  requestUpdate,
  checkForUpdates,
  type UpdateStatus,
  type UpdateEntry,
  type UpdateCheck,
} from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";

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
  const [available, setAvailable] = useState<UpdateCheck | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [requesting, setRequesting] = useState(false);
  const [requestMessage, setRequestMessage] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  const refresh = useCallback(() => setRevision((v) => v + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load(includeReleaseCheck: boolean) {
      try {
        const [s, h, c] = await Promise.all([
          loadUpdateStatus(controller.signal),
          loadUpdateHistory(controller.signal),
          includeReleaseCheck ? checkForUpdates(controller.signal) : Promise.resolve(null),
        ]);
        if (controller.signal.aborted) return;
        setStatus(s);
        setHistory(h);
        if (c) setAvailable(c);
        setError(null);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : t("updates.load_failed"));
      } finally {
        setLoading(false);
        if (!controller.signal.aborted) timer = setTimeout(() => void load(false), 10000);
      }
    }
    void load(true);
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
    return (
      <section className="panel">
        <Loading>{t("updates.loading")}</Loading>
      </section>
    );
  }

  const phases = ["downloading", "verifying", "installing", "readiness"];
  const failed = status?.state === "failed" || status?.state === "rolled_back";
  const heroTone = status?.updating ? "" : status?.state === "completed" ? " success" : failed ? " danger" : "";

  return (
    <div className="stack">
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("updates.current_status")}</h2>
          </div>
          {!readOnly && (
            <div className="button-group">
              <button className="button" onClick={refresh} disabled={requesting || status?.updating}>
                <RefreshCw size={15} /> {t("updates.check")}
              </button>
              <button
                className="button primary"
                onClick={handleRequestUpdate}
                disabled={requesting || status?.updating || !available?.update_available}
              >
                <Download size={15} />
                {requesting ? t("updates.requesting") : status?.updating ? t("updates.in_progress") : t("updates.request")}
              </button>
            </div>
          )}
        </div>
        <div className="panel-body stack-sm">
          {error && (
            <div className="notice error" role="alert">
              {error}
            </div>
          )}
          {status && (
            <div className="version-hero">
              <span className={`version-icon${heroTone}`} aria-hidden="true">
                {status.updating ? (
                  <Clock size={22} className="spin" />
                ) : status.state === "completed" ? (
                  <CheckCircle size={22} />
                ) : failed ? (
                  <XCircle size={22} />
                ) : (
                  <Download size={22} />
                )}
              </span>
              <div>
                <strong>
                  {status.updating
                    ? `${t("updates.updating")} ${stateLabel(status.state)}`
                    : `${t("updates.installed")} ${status.installed}`}
                </strong>
                {status.updating && status.from_version && status.to_version ? (
                  <span>
                    {status.from_version} &rarr; {status.to_version}
                  </span>
                ) : (
                  status.last_completed && (
                    <span>
                      {t("updates.last_completed")} {new Date(status.last_completed).toLocaleString()}
                    </span>
                  )
                )}
              </div>
            </div>
          )}
          {status?.updating && (
            <ol className="update-steps" aria-label={t("updates.progress")}>
              {phases.map((phase) => {
                const current = phases.indexOf(status.state);
                const index = phases.indexOf(phase);
                const state = index < current ? "done" : index === current ? "current" : "";
                return (
                  <li key={phase} className={state}>
                    {index < current ? "✓" : index === current ? "●" : "○"} {stateLabel(phase)}
                  </li>
                );
              })}
            </ol>
          )}
          {status?.error && <div className="notice error" role="alert">{status.error}</div>}
          {status?.rollback_used && (
            <div className="notice warning" role="status">
              {status.readiness_ok ? t("updates.rollback_recovered") : t("updates.rollback_used")}
            </div>
          )}
          {available && (
            <div className={`release-card${available.update_available ? " available" : ""}`}>
              <strong>
                {available.update_available
                  ? `${t("updates.available")}: ${available.latest_version}`
                  : t("updates.up_to_date")}
              </strong>
              <div className="release-meta">
                <span>{t("updates.channel")}: {available.channel} · {available.architecture}</span>
                {available.release_date && <span>{t("updates.release_date")}: {new Date(available.release_date).toLocaleDateString()}</span>}
                {available.download_size && <span>{t("updates.download_size")}: {(available.download_size / 1024 / 1024).toFixed(1)} MiB</span>}
              </div>
              {available.release_notes && <p className="release-notes">{available.release_notes}</p>}
            </div>
          )}
          {readOnly && (
            <div className="notice info" role="status">
              {t("updates.viewer_readonly")}
            </div>
          )}
          {requestMessage && (
            <div className="notice" role="status">
              <AlertTriangle size={15} /> {requestMessage}
            </div>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("updates.history")}</h2>
          </div>
        </div>
        {history.length === 0 ? (
          <EmptyState compact icon={<History size={22} />} title={t("updates.no_history")} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("updates.col_from")}</th>
                  <th>{t("updates.col_to")}</th>
                  <th>{t("updates.col_channel")}</th>
                  <th>{t("updates.col_state")}</th>
                  <th>{t("updates.col_mode")}</th>
                  <th>{t("updates.col_started")}</th>
                  <th className="num">{t("updates.col_duration")}</th>
                </tr>
              </thead>
              <tbody>
                {history.map((entry) => (
                  <tr key={entry.id}>
                    <td className="mono cell-muted">{entry.from_version}</td>
                    <td className="mono">{entry.to_version}</td>
                    <td>
                      <Badge
                        plain
                        tone={entry.channel === "beta" ? "info" : entry.channel === "alpha" ? "warning" : "brand"}
                      >
                        {entry.channel || "stable"}
                      </Badge>
                    </td>
                    <td className="wrap">
                      <Badge tone={toneFor(entry.state)}>{stateLabel(entry.state)}</Badge>
                      {entry.rollback_used && <span className="cell-muted"> · {t("updates.rolled_back")}</span>}
                      {entry.error && <span className="error-text">{entry.error}</span>}
                    </td>
                    <td className="cell-muted">{entry.deployment_mode}</td>
                    <td className="cell-muted">{new Date(entry.started_at).toLocaleString()}</td>
                    <td className="num mono">
                      {entry.completed_at
                        ? `${(
                            (new Date(entry.completed_at).getTime() - new Date(entry.started_at).getTime()) /
                            1000
                          ).toFixed(1)}s`
                        : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
