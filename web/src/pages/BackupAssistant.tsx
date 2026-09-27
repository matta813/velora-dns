import { Database, CheckCircle, XCircle, AlertTriangle, Clock, Download } from "lucide-react";
import { useEffect, useState } from "react";
import { downloadEncryptedBackup, request } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";

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
    return (
      <section className="panel">
        <Loading>{t("backup.loading")}</Loading>
      </section>
    );
  }

  return (
    <div className="stack">
      <div className="grid-2">
        <section className="panel backup-create-panel">
          <div className="panel-heading">
            <div>
              <h2>{t("backup.create_title")}</h2>
              <p>{t("backup.create_hint")}</p>
            </div>
          </div>
          <div className="panel-body">
            {canCreate && status && !status.supported && <p className="notice">{t("backup.sqlite_only")}</p>}
            {canCreate && status?.supported && (
              <div className="inline-form">
                <div className="field">
                  <label htmlFor="backup-passphrase">{t("backup.passphrase")}</label>
                  <input
                    id="backup-passphrase"
                    type="password"
                    autoComplete="new-password"
                    value={passphrase}
                    onChange={(event) => setPassphrase(event.target.value)}
                  />
                </div>
                <button
                  className="button primary"
                  disabled={creating || passphrase.length < 12}
                  onClick={() => void createBackup()}
                >
                  <Download size={15} />
                  {creating ? t("backup.creating") : t("backup.create")}
                </button>
              </div>
            )}
            {createError && <div className="notice error" role="alert">{createError}</div>}
          </div>
        </section>

        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("backup.status_title")}</h2>
            </div>
          </div>
          <div className="panel-body">
            {error && (
              <div className="notice error" role="alert">
                {error}
              </div>
            )}
            {status && (
              <div className="status-rows">
                <div className="status-row">
                  <Database size={17} />
                  <span className="value">
                    {t("backup.database")} <code>{status.database_path}</code>
                  </span>
                </div>
                {status.last_backup_time ? (
                  <div className="status-row ok">
                    <CheckCircle size={17} />
                    <span className="value">
                      {t("backup.last_backup")} {new Date(status.last_backup_time).toLocaleString()}
                      {status.backup_age && ` (${status.backup_age})`}
                    </span>
                  </div>
                ) : (
                  <div className="status-row warn">
                    <AlertTriangle size={17} />
                    <span className="value">{t("backup.no_backup")}</span>
                  </div>
                )}
                <div className="status-row">
                  <Clock size={17} />
                  <span className="value">
                    {t("backup.verification_state")}{" "}
                    <Badge tone={toneFor(status.verification_state)}>{statusLabel(status.verification_state)}</Badge>
                  </span>
                </div>
              </div>
            )}
          </div>
        </section>
      </div>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("backup.verify_title")}</h2>
            <p>{t("backup.verify_text")}</p>
          </div>
        </div>
        <div className="panel-body">
          {readOnly ? (
            <div className="notice info" role="status">
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
                  <button className="button" onClick={handleVerify} disabled={verifying || !verifyPath.trim()}>
                    {verifying ? t("backup.verifying") : t("backup.verify")}
                  </button>
                </div>
              </div>
              <p className="backup-file-hint">{t("backup.file_hint")}</p>
            </div>
          )}
          {verificationResult && (
            <div className={`result-card ${verificationResult.valid ? "valid" : "invalid"}`}>
              {verificationResult.valid ? <CheckCircle size={18} /> : <XCircle size={18} />}
              <div>
                <strong>{verificationResult.valid ? t("backup.valid") : t("backup.invalid")}</strong>
                <p>
                  {verificationResult.valid
                    ? `${t("backup.schema_version")} ${verificationResult.schema_version} · ${t("backup.records")} ${verificationResult.record_count}`
                    : verificationResult.error}
                </p>
              </div>
            </div>
          )}
        </div>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("backup.instructions")}</h2>
            <p>{t("backup.restore_hint")}</p>
          </div>
        </div>
        <div className="panel-body">
          <pre className="backup-restore-command">{`sudo systemctl stop velora-dns
read -rsp 'Backup passphrase: ' VELORA_BACKUP_PASSPHRASE; export VELORA_BACKUP_PASSPHRASE
sudo -E velora-dns -restore-backup ./velora-backup.vdns -config /etc/velora/config.yaml -restore-database /var/lib/velora/velora.db
sudo systemctl start velora-dns`}</pre>
          <details className="recipes">
            <summary>{t("backup.native")} · {t("backup.docker")} · {t("backup.online")}</summary>
            <div className="recipe">
              <h3>{t("backup.native")}</h3>
              <pre className="code-block">
{`# ${t("backup.step_stop_service")}
sudo systemctl stop velora-dns

# ${t("backup.step_create_backup")}
sqlite3 /var/lib/velora/velora.db ".backup /var/lib/velora/backup-$(date +%Y%m%d).db"

# ${t("backup.step_restart_service")}
sudo systemctl start velora-dns`}
              </pre>
            </div>
            <div className="recipe">
              <h3>{t("backup.docker")}</h3>
              <pre className="code-block">
{`# ${t("backup.step_stop_container")}
docker compose stop velora

# ${t("backup.step_backup_volume")}
docker run --rm -v velora-dns_velora-data:/data -v $(pwd):/backup \\
  alpine tar czf /backup/velora-data-$(date +%Y%m%d).tar.gz -C /data .

# ${t("backup.step_restart_container")}
docker compose start velora`}
              </pre>
            </div>
            <div className="recipe">
              <h3>{t("backup.online")}</h3>
              <pre className="code-block">
{`# ${t("backup.step_online_backup")}
sqlite3 /var/lib/velora/velora.db \\
  ".backup /var/lib/velora/online-backup.db"`}
              </pre>
            </div>
          </details>
        </div>
      </section>
    </div>
  );
}
