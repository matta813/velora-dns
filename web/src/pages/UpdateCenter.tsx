import { CheckCircle2, Download, History, Loader2, RefreshCw, RotateCcw, XCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  checkForUpdates,
  loadUpdateHistory,
  loadUpdateStatus,
  requestUpdate,
  type UpdateCheck,
  type UpdateEntry,
  type UpdateStatus,
} from "../api";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";
import { useI18n } from "../i18n-context";

interface Props {
  readOnly: boolean;
  /** Called once an update completes so the app can reload its version. */
  onUpdated?: () => void;
}

/**
 * The page walks through one path: check → (up to date | update available) →
 * updating → completed or failed. Only one primary action is ever shown.
 */
type Phase = "idle" | "checking" | "up_to_date" | "available" | "starting" | "updating" | "completed" | "failed";

const STEPS = ["downloading", "verifying", "installing", "readiness"] as const;
const POLL_MS = 2000;
const START_TIMEOUT_MS = 30000;

export function UpdateCenter({ readOnly, onUpdated }: Props) {
  const { t, language } = useI18n();
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [check, setCheck] = useState<UpdateCheck | null>(null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [reconnecting, setReconnecting] = useState(false);
  const [history, setHistory] = useState<UpdateEntry[] | null>(null);
  const notified = useRef(false);
  const sawUpdate = useRef(false);
  const startedAt = useRef(0);

  const stateLabel = (state: string) => {
    const key = `updates.state_${state}`;
    const translated = t(key);
    return translated === key ? state : translated;
  };

  // Initial status: only resume progress display if an update is already running.
  useEffect(() => {
    const controller = new AbortController();
    loadUpdateStatus(controller.signal)
      .then((next) => {
        if (controller.signal.aborted) return;
        setStatus(next);
        if (next.updating) {
          sawUpdate.current = true;
          setPhase("updating");
        }
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(e instanceof Error ? e.message : t("updates.load_failed"));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [t]);

  // While updating, poll the agent. The web server restarts during the
  // update, so failed polls mean "reconnecting", not an error.
  useEffect(() => {
    if (phase !== "starting" && phase !== "updating") return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await loadUpdateStatus();
        if (cancelled) return;
        setReconnecting(false);
        setStatus(next);
        const finishedSinceRequest = Boolean(next.last_completed && new Date(next.last_completed).getTime() >= startedAt.current);
        if (next.updating) {
          sawUpdate.current = true;
          setPhase("updating");
        } else if (!sawUpdate.current && !finishedSinceRequest) {
          // The last run's final state is still reported until the agent
          // picks up the new request, so wait for it to start.
          if (Date.now() - startedAt.current > START_TIMEOUT_MS) {
            setError(t("updates.not_started"));
            setPhase("failed");
            return;
          }
        } else if (next.state === "completed") {
          setPhase("completed");
          if (!notified.current) {
            notified.current = true;
            onUpdated?.();
          }
          return;
        } else if (next.state === "failed" || next.state === "rolled_back") {
          setPhase("failed");
          return;
        } else {
          setPhase("idle");
          return;
        }
      } catch {
        if (!cancelled) setReconnecting(true);
      }
      if (!cancelled) timer = setTimeout(() => void poll(), POLL_MS);
    };
    timer = setTimeout(() => void poll(), phase === "starting" ? 500 : POLL_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [phase, onUpdated, t]);

  const runCheck = useCallback(async () => {
    setPhase("checking");
    setError("");
    try {
      const result = await checkForUpdates();
      setCheck(result);
      setPhase(result.update_available ? "available" : "up_to_date");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("updates.load_failed"));
      setPhase("idle");
    }
  }, [t]);

  const startUpdate = async () => {
    setError("");
    notified.current = false;
    sawUpdate.current = false;
    startedAt.current = Date.now();
    setPhase("starting");
    try {
      await requestUpdate();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("updates.request_failed"));
      setPhase(check?.update_available ? "available" : "idle");
    }
  };

  const toggleHistory = (open: boolean) => {
    if (open && history === null) {
      loadUpdateHistory()
        .then(setHistory)
        .catch(() => setHistory([]));
    }
  };

  if (loading) {
    return (
      <section className="panel">
        <Loading>{t("updates.loading")}</Loading>
      </section>
    );
  }

  const busy = phase === "checking" || phase === "starting" || phase === "updating";
  const installed = status?.installed || check?.installed_version || "—";
  const currentStep = status ? STEPS.indexOf(status.state as (typeof STEPS)[number]) : -1;

  return (
    <div className="stack">
      <section className="panel updater">
        <div className="updater-body">
          <span className={`updater-icon ${phase}`} aria-hidden="true">
            {phase === "completed" || phase === "up_to_date" ? <CheckCircle2 size={28} />
              : phase === "failed" ? <XCircle size={28} />
              : busy ? <Loader2 size={28} className="spin" />
              : <Download size={28} />}
          </span>
          <div className="updater-text">
            <p className="updater-version">
              Velora DNS <strong>{installed}</strong>
            </p>
            <h2 aria-live="polite">
              {phase === "idle" && t("updates.check_prompt")}
              {phase === "checking" && t("updates.checking")}
              {phase === "up_to_date" && t("updates.up_to_date")}
              {phase === "available" && `${t("updates.available")}: ${check?.latest_version}`}
              {phase === "starting" && t("updates.starting")}
              {phase === "updating" && (status?.to_version ? `${t("updates.updating_to")} ${status.to_version}` : t("updates.in_progress"))}
              {phase === "completed" && `${t("updates.completed_to")} ${status?.installed ?? ""}`}
              {phase === "failed" && t("updates.failed_title")}
            </h2>
            {phase === "available" && check && (
              <p className="updater-meta">
                {check.channel} · {check.architecture}
                {check.release_date && ` · ${t("updates.release_date")} ${new Date(check.release_date).toLocaleDateString(language)}`}
                {check.download_size ? ` · ${(check.download_size / 1024 / 1024).toFixed(1)} MiB` : ""}
              </p>
            )}
            {phase === "up_to_date" && check && <p className="updater-meta">{check.channel} · {check.architecture}</p>}
          </div>
          <div className="updater-actions">
            {(phase === "idle" || phase === "checking" || phase === "up_to_date" || phase === "failed") && (
              <button className={`button${phase === "up_to_date" ? "" : " primary"}`} onClick={() => void runCheck()} disabled={busy}>
                <RefreshCw size={15} className={phase === "checking" ? "spin" : undefined} />
                {phase === "failed" ? t("updates.check_again") : t("updates.check")}
              </button>
            )}
            {phase === "available" && !readOnly && (
              <button className="button primary" onClick={() => void startUpdate()}>
                <Download size={15} />
                {t("updates.update_now")}
              </button>
            )}
            {phase === "completed" && (
              <button className="button primary" onClick={() => window.location.reload()}>
                <RotateCcw size={15} />
                {t("updates.reload")}
              </button>
            )}
          </div>
        </div>

        {phase === "available" && check?.release_notes && <p className="release-notes updater-notes">{check.release_notes}</p>}
        {phase === "available" && readOnly && <div className="notice info updater-notice" role="status">{t("updates.viewer_readonly")}</div>}

        {(phase === "starting" || phase === "updating") && (
          <div className="updater-progress">
            <ol className="update-steps" aria-label={t("updates.progress")}>
              {STEPS.map((step, index) => {
                const state = index < currentStep ? "done" : index === currentStep ? "current" : "";
                return (
                  <li key={step} className={state}>
                    {index < currentStep ? "✓" : index === currentStep ? "●" : "○"} {stateLabel(step)}
                  </li>
                );
              })}
            </ol>
            <p className="updater-meta" role="status">
              {reconnecting ? t("updates.reconnecting") : t("updates.keep_open")}
            </p>
          </div>
        )}

        {phase === "completed" && <div className="notice success updater-notice" role="status">{t("updates.completed_text")}</div>}
        {phase === "failed" && (
          <div className="notice error updater-notice" role="alert">
            {status?.error || t("updates.failed_text")}
            {status?.rollback_used && <> {status.readiness_ok ? t("updates.rollback_recovered") : t("updates.rollback_used")}</>}
          </div>
        )}
        {error && <div className="notice error updater-notice" role="alert">{error}</div>}
      </section>

      <details className="panel updater-history" onToggle={(event) => toggleHistory(event.currentTarget.open)}>
        <summary>
          <History size={15} /> {t("updates.history")}
        </summary>
        {history === null ? (
          <Loading>{t("updates.loading")}</Loading>
        ) : history.length === 0 ? (
          <EmptyState compact title={t("updates.no_history")} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("updates.col_to")}</th>
                  <th>{t("updates.col_from")}</th>
                  <th>{t("updates.col_state")}</th>
                  <th>{t("updates.col_started")}</th>
                </tr>
              </thead>
              <tbody>
                {history.map((entry) => (
                  <tr key={entry.id}>
                    <td className="mono">{entry.to_version}</td>
                    <td className="mono cell-muted">{entry.from_version}</td>
                    <td className="wrap">
                      <Badge tone={toneFor(entry.state)}>{stateLabel(entry.state)}</Badge>
                      {entry.error && <span className="error-text">{entry.error}</span>}
                    </td>
                    <td className="cell-muted">{new Date(entry.started_at).toLocaleString(language)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </details>
    </div>
  );
}
