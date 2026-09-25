import { Database, CheckCircle, XCircle, AlertTriangle, Clock } from "lucide-react";
import { useEffect, useState } from "react";
import { downloadEncryptedBackup, request } from "../api";
import { useI18n } from "../i18n-context";

interface BackupStatus {
  supported: boolean;
  last_backup_time?: string;
  last_backup_size?: number;
  backup_age?: string;
  verification_state: string;
  database_path: string;
}

interface BackupVerification {
  valid: boolean;
  schema_version: number;
  record_count: number;
  error?: string;
}

interface Props {
  readOnly: boolean;
  canCreate: boolean;
}

export function BackupAssistant({ readOnly, canCreate }: Props) {
  const { t } = useI18n();
  const statusLabel = (state: string) => {
    const key = `backup.state_${state}`;
    const translated = t(key);
    return translated === key ? state : translated;
  };
  const [status, setStatus] = useState<BackupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [verifyPath, setVerifyPath] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verificationResult, setVerificationResult] = useState<BackupVerification | null>(null);
  const [passphrase, setPassphrase] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  const createBackup = async () => {
    setCreating(true);
    setCreateError("");
    try {
      const archive = await downloadEncryptedBackup(passphrase);
      const url = URL.createObjectURL(archive);
      const link = document.createElement("a");
      link.href = url;
      link.download = `velora-backup-${new Date().toISOString().slice(0, 10)}.vdns`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setPassphrase("");
    } catch (reason) {
      setCreateError(reason instanceof Error ? reason.message : t("backup.create_failed"));
    } finally {
      setCreating(false);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const s = await request<BackupStatus>("/api/v1/backup/status", controller.signal);
        if (controller.signal.aborted) return;
        setStatus(s);
        setError(null);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : t("backup.load_failed"));
      } finally {
        setLoading(false);
      }
    }
    void load();
    return () => controller.abort();
  }, [t]);

  const handleVerify = async () => {
    if (!verifyPath.trim()) return;
    setVerifying(true);
    setVerificationResult(null);
    try {
      const result = await request<BackupVerification>("/api/v1/backup/verify", undefined, "POST", {
        body: { path: verifyPath },
      });
      setVerificationResult(result);
    } catch (e) {
      setVerificationResult({
        valid: false,
        schema_version: 0,
        record_count: 0,
        error: e instanceof Error ? e.message : t("backup.verify_failed"),
      });
    } finally {
      setVerifying(false);
    }
  };

  if (loading) {
    return <div className="panel padded">{t("backup.loading")}</div>;
  }

  return (
    <div>
      <section className="panel padded backup-create-panel">
        <h2>{t("backup.create_title")}</h2>
        <p>{t("backup.create_hint")}</p>
        {canCreate && status && !status.supported && <p>{t("backup.sqlite_only")}</p>}
        {canCreate && status?.supported && <div className="backup-file-controls">
          <label htmlFor="backup-passphrase">{t("backup.passphrase")}</label>
          <input id="backup-passphrase" type="password" autoComplete="new-password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} />
          <button className="button" disabled={creating || passphrase.length < 12} onClick={() => void createBackup()}>{creating ? t("backup.creating") : t("backup.create")}</button>
        </div>}
        {createError && <div className="notice error" role="alert">{createError}</div>}
      </section>
      <div className="panel padded">
        <h2>{t("backup.status_title")}</h2>
        {error && (
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
        {status && (
          <div style={{ display: "grid", gap: "1rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <Database size={18} />
              <strong>{t("backup.database")} {status.database_path}</strong>
            </div>
            {status.last_backup_time ? (
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />
                <span>
                  {t("backup.last_backup")} {new Date(status.last_backup_time).toLocaleString()}
                  {status.backup_age && ` (${status.backup_age})`}
                </span>
              </div>
            ) : (
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <AlertTriangle size={18} style={{ color: "var(--warning, #f59e0b)" }} />
                <span>{t("backup.no_backup")}</span>
              </div>
            )}
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <Clock size={18} />
              <span>{t("backup.verification_state")} {statusLabel(status.verification_state)}</span>
            </div>
          </div>
        )}
      </div>

      <div className="panel padded" style={{ marginTop: "1rem" }}>
        <h2>{t("backup.verify_title")}</h2>
        <p style={{ opacity: 0.7, marginBottom: "1rem" }}>
          {t("backup.verify_text")}
        </p>
        {readOnly ? (
          <div className="notice" role="status">
            {t("backup.viewer_readonly")}
          </div>
        ) : (
          <div>
            <div className="backup-file-label">
              <label htmlFor="backup-file-name">{t("backup.file_name")}</label>
              <div className="backup-file-controls">
                <input
                  id="backup-file-name"
                  type="text"
                  placeholder="backup.db"
                  value={verifyPath}
                  onChange={(e) => setVerifyPath(e.target.value)}
                />
                <button
                  className="button"
                  onClick={handleVerify}
                  disabled={verifying || !verifyPath.trim()}
                >
                  {verifying ? t("backup.verifying") : t("backup.verify")}
                </button>
              </div>
            </div>
            <p className="backup-file-hint">{t("backup.file_hint")}</p>
          </div>
        )}
        {verificationResult && (
          <div
            style={{
              marginTop: "1rem",
              padding: "1rem",
              borderRadius: "6px",
              backgroundColor: verificationResult.valid
                ? "rgba(34, 197, 94, 0.05)"
                : "rgba(239, 68, 68, 0.05)",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              {verificationResult.valid ? (
                <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />
              ) : (
                <XCircle size={18} style={{ color: "var(--error, #ef4444)" }} />
              )}
              <strong>{verificationResult.valid ? t("backup.valid") : t("backup.invalid")}</strong>
            </div>
            {verificationResult.valid ? (
              <div style={{ marginTop: "0.5rem", fontSize: "0.85rem", opacity: 0.7 }}>
                {t("backup.schema_version")} {verificationResult.schema_version} | {t("backup.records")}{" "}
                {verificationResult.record_count}
              </div>
            ) : (
              <div style={{ marginTop: "0.5rem", fontSize: "0.85rem", color: "var(--error, #ef4444)" }}>
                {verificationResult.error}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="panel padded" style={{ marginTop: "1rem" }}>
        <h2>{t("backup.instructions")}</h2>
        <p>{t("backup.restore_hint")}</p>
        <pre className="backup-restore-command">{`sudo systemctl stop velora-dns
read -rsp 'Backup passphrase: ' VELORA_BACKUP_PASSPHRASE; export VELORA_BACKUP_PASSPHRASE
sudo -E velora-dns -restore-backup ./velora-backup.vdns -config /etc/velora/config.yaml -restore-database /var/lib/velora/velora.db
sudo systemctl start velora-dns`}</pre>
        <div style={{ display: "grid", gap: "1rem", fontSize: "0.9rem" }}>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>{t("backup.native")}</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# ${t("backup.step_stop_service")}
sudo systemctl stop velora-dns

# ${t("backup.step_create_backup")}
sqlite3 /var/lib/velora/velora.db ".backup /var/lib/velora/backup-$(date +%Y%m%d).db"

# ${t("backup.step_restart_service")}
sudo systemctl start velora-dns`}
            </pre>
          </div>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>{t("backup.docker")}</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# ${t("backup.step_stop_container")}
docker compose stop velora

# ${t("backup.step_backup_volume")}
docker run --rm -v velora-dns_velora-data:/data -v $(pwd):/backup \\
  alpine tar czf /backup/velora-data-$(date +%Y%m%d).tar.gz -C /data .

# ${t("backup.step_restart_container")}
docker compose start velora`}
            </pre>
          </div>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>{t("backup.online")}</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# ${t("backup.step_online_backup")}
sqlite3 /var/lib/velora/velora.db \\
  ".backup /var/lib/velora/online-backup.db"`}
            </pre>
          </div>
        </div>
      </div>
    </div>
  );
}
