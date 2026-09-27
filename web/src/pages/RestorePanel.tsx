import { AlertTriangle, CheckCircle, FileSearch, Loader2, RotateCcw, Upload, XCircle } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { APIError, inspectBackup, loadRestoreState, scheduleRestore, type BackupInspection, type RestoreState } from "../api";
import { useI18n } from "../i18n-context";

type Phase = "idle" | "inspecting" | "inspected" | "scheduling" | "restarting" | "signed_out" | "finished";

const POLL_MS = 2000;

/**
 * Restore flow: upload and inspect (nothing changes), confirm, then the
 * server restarts, applies the backup and reports the result.
 */
export function RestorePanel() {
  const { t, language } = useI18n();
  const [file, setFile] = useState<File | null>(null);
  const [passphrase, setPassphrase] = useState("");
  const [phase, setPhase] = useState<Phase>("idle");
  const [inspection, setInspection] = useState<BackupInspection | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<RestoreState | null>(null);
  const scheduledAt = useRef(0);
  const fileInput = useRef<HTMLInputElement>(null);

  // Show the outcome of the most recent restore, if any.
  useEffect(() => {
    const controller = new AbortController();
    loadRestoreState(controller.signal)
      .then((state) => {
        if (!controller.signal.aborted && state?.metadata && state.state !== "pending") setResult(state);
      })
      .catch(() => undefined);
    return () => controller.abort();
  }, []);

  // After scheduling, the server restarts; wait for it and read the result.
  useEffect(() => {
    if (phase !== "restarting") return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const state = await loadRestoreState();
        if (cancelled) return;
        const fresh = state && new Date(state.updated_at).getTime() >= scheduledAt.current - 5000;
        if (state && fresh && ["completed", "rolled_back", "failed"].includes(state.state)) {
          setResult(state);
          setPhase("finished");
          return;
        }
      } catch (e) {
        // Sessions come from the restored database, so ours may be gone.
        if (!cancelled && e instanceof APIError && e.status === 401) {
          setPhase("signed_out");
          return;
        }
      }
      if (!cancelled) timer = setTimeout(() => void poll(), POLL_MS);
    };
    timer = setTimeout(() => void poll(), POLL_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [phase]);

  async function inspect(event: FormEvent) {
    event.preventDefault();
    if (!file) return;
    setPhase("inspecting");
    setError("");
    setInspection(null);
    setConfirmed(false);
    try {
      setInspection(await inspectBackup(file, passphrase));
      setPhase("inspected");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("restore.inspect_failed"));
      setPhase("idle");
    }
  }

  async function restore() {
    if (!inspection) return;
    setPhase("scheduling");
    setError("");
    try {
      scheduledAt.current = Date.now();
      const scheduled = await scheduleRestore(inspection.token);
      setPassphrase("");
      setResult(null);
      setPhase(scheduled.state === "pending" ? "finished" : "restarting");
      if (scheduled.state === "pending") setError(t("restore.manual_restart"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("restore.schedule_failed"));
      setPhase("inspected");
    }
  }

  const reset = () => {
    setPhase("idle");
    setInspection(null);
    setFile(null);
    setConfirmed(false);
    if (fileInput.current) fileInput.current.value = "";
  };

  const summary = inspection?.summary;
  return (
    <section className="panel restore-panel">
      <div className="panel-heading">
        <div>
          <h2>{t("restore.title")}</h2>
          <p>{t("restore.hint")}</p>
        </div>
      </div>
      <div className="panel-body stack-sm">
        {result && phase !== "restarting" && (
          <div className={`notice ${result.state === "completed" ? "success" : "error"}`} role="status">
            {result.state === "completed" ? <CheckCircle size={16} /> : <XCircle size={16} />}
            <span>
              <strong>{t(`restore.state_${result.state}`)}</strong>{" "}
              {t("restore.backup_from")} {new Date(result.metadata.created_at).toLocaleString(language)} ({result.metadata.velora_version})
              {" · "}{new Date(result.updated_at).toLocaleString(language)}
              {result.error && <span className="restore-error">{result.error}</span>}
              {result.safety_copy && <small className="restore-safety">{t("restore.safety_copy")} <code>{result.safety_copy}</code></small>}
            </span>
          </div>
        )}

        {(phase === "idle" || phase === "inspecting") && (
          <form className="restore-form" onSubmit={(event) => void inspect(event)}>
            <label className="field">
              <span>{t("restore.file")}</span>
              <input ref={fileInput} type="file" accept=".vdns,application/octet-stream" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
            </label>
            <label className="field">
              <span>{t("restore.passphrase")}</span>
              <input type="password" autoComplete="off" value={passphrase} onChange={(e) => setPassphrase(e.target.value)} minLength={12} required />
            </label>
            <button className="button" type="submit" disabled={phase === "inspecting" || !file || passphrase.length < 12}>
              {phase === "inspecting" ? <Loader2 size={15} className="spin" /> : <FileSearch size={15} />}
              {phase === "inspecting" ? t("restore.inspecting") : t("restore.inspect")}
            </button>
          </form>
        )}

        {inspection && summary && (phase === "inspected" || phase === "scheduling") && (
          <div className="restore-review">
            <h3>{t("restore.review_title")}</h3>
            <dl className="restore-facts">
              <dt>{t("restore.created")}</dt>
              <dd>{new Date(inspection.metadata.created_at).toLocaleString(language)}</dd>
              <dt>{t("restore.version")}</dt>
              <dd><code>{inspection.metadata.velora_version}</code> → <code>{summary.current_version}</code></dd>
              <dt>{t("restore.schema")}</dt>
              <dd>{inspection.metadata.schema_version} → {summary.current_schema}</dd>
              <dt>{t("restore.contents")}</dt>
              <dd>
                {[
                  [summary.zones, t("restore.zones")],
                  [summary.records, t("restore.records")],
                  [summary.blocklists, t("restore.blocklists")],
                  [summary.rewrites, t("restore.rewrites")],
                  [summary.forward_rules, t("restore.forward_rules")],
                  [summary.clients, t("restore.clients")],
                  [summary.users, t("restore.users")],
                ].map(([count, label]) => `${count} ${label}`).join(" · ")}
              </dd>
              <dt>{t("restore.listeners")}</dt>
              <dd><code>{summary.dns_listen.join(", ")}</code> · {t("restore.web")} <code>{summary.http_listen}</code></dd>
            </dl>
            {summary.warnings.map((warning) => (
              <p key={warning} className="notice warning"><AlertTriangle size={15} /> {warning}</p>
            ))}
            <label className="check-field">
              <input type="checkbox" checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} />
              <span>{t("restore.confirm")}</span>
            </label>
            <div className="form-actions">
              <button className="button danger" disabled={!confirmed || phase === "scheduling"} onClick={() => void restore()}>
                <Upload size={15} />
                {t("restore.restore")}
              </button>
              <button className="button" type="button" onClick={reset} disabled={phase === "scheduling"}>{t("forwarding.cancel")}</button>
            </div>
          </div>
        )}

        {phase === "restarting" && (
          <div className="notice info" role="status">
            <Loader2 size={16} className="spin" /> {t("restore.restarting")}
          </div>
        )}
        {phase === "signed_out" && (
          <div className="notice success" role="status">
            <CheckCircle size={16} /> {t("restore.signed_out")}
            <button className="button small" onClick={() => window.location.reload()}><RotateCcw size={14} /> {t("updates.reload")}</button>
          </div>
        )}
        {phase === "finished" && (
          <button className="button" onClick={reset}>{t("restore.again")}</button>
        )}
        {error && <div className="notice error" role="alert">{error}</div>}
      </div>
    </section>
  );
}
