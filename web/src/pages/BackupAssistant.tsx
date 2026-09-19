import { Database, CheckCircle, XCircle, AlertTriangle, Clock } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import { request } from "../api";

interface BackupStatus {
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
}

export function BackupAssistant({ readOnly }: Props) {
  const [status, setStatus] = useState<BackupStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [verifyPath, setVerifyPath] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verificationResult, setVerificationResult] = useState<BackupVerification | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await request<BackupStatus>("/api/v1/backup/status");
      setStatus(s);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load backup status");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

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
        error: e instanceof Error ? e.message : "Verification failed",
      });
    } finally {
      setVerifying(false);
    }
  };

  if (loading) {
    return <div className="panel padded">Loading backup status...</div>;
  }

  return (
    <div>
      <div className="panel padded">
        <h2>Backup Status</h2>
        {error && (
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
        {status && (
          <div style={{ display: "grid", gap: "1rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <Database size={18} />
              <strong>Database: {status.database_path}</strong>
            </div>
            {status.last_backup_time ? (
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />
                <span>
                  Last backup: {new Date(status.last_backup_time).toLocaleString()}
                  {status.backup_age && ` (${status.backup_age})`}
                </span>
              </div>
            ) : (
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <AlertTriangle size={18} style={{ color: "var(--warning, #f59e0b)" }} />
                <span>No backup recorded yet</span>
              </div>
            )}
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <Clock size={18} />
              <span>Verification state: {status.verification_state}</span>
            </div>
          </div>
        )}
      </div>

      <div className="panel padded" style={{ marginTop: "1rem" }}>
        <h2>Verify Backup</h2>
        <p style={{ opacity: 0.7, marginBottom: "1rem" }}>
          Restore a backup into an isolated temporary location and verify its integrity.
        </p>
        {readOnly ? (
          <div className="notice" role="status">
            You are signed in as a viewer. Backup verification is read-only.
          </div>
        ) : (
          <div style={{ display: "flex", gap: "0.5rem" }}>
            <input
              type="text"
              placeholder="/path/to/backup.db"
              value={verifyPath}
              onChange={(e) => setVerifyPath(e.target.value)}
              style={{ flex: 1, padding: "0.5rem", border: "1px solid var(--border, #374151)", borderRadius: "4px", backgroundColor: "var(--bg, #1f2937)", color: "var(--text, #f9fafb)" }}
            />
            <button
              className="button"
              onClick={handleVerify}
              disabled={verifying || !verifyPath.trim()}
            >
              {verifying ? "Verifying..." : "Verify"}
            </button>
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
              <strong>{verificationResult.valid ? "Backup is valid" : "Backup is invalid"}</strong>
            </div>
            {verificationResult.valid ? (
              <div style={{ marginTop: "0.5rem", fontSize: "0.85rem", opacity: 0.7 }}>
                Schema version: {verificationResult.schema_version} | Records:{" "}
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
        <h2>Backup Instructions</h2>
        <div style={{ display: "grid", gap: "1rem", fontSize: "0.9rem" }}>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>Native Installation</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# Stop the service
sudo systemctl stop velora-dns

# Create a consistent backup
sqlite3 /var/lib/velora/velora.db ".backup /var/lib/velora/backup-$(date +%Y%m%d).db"

# Restart the service
sudo systemctl start velora-dns`}
            </pre>
          </div>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>Docker Compose</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# Stop the container
docker compose stop velora

# Backup the volume
docker run --rm -v velora-dns_velora-data:/data -v $(pwd):/backup \\
  alpine tar czf /backup/velora-data-$(date +%Y%m%d).tar.gz -C /data .

# Restart the container
docker compose start velora`}
            </pre>
          </div>
          <div>
            <h3 style={{ marginBottom: "0.5rem" }}>Online Backup (SQLite)</h3>
            <pre style={{ padding: "1rem", borderRadius: "6px", backgroundColor: "var(--code-bg, #111827)", overflow: "auto" }}>
{`# Use SQLite backup API for consistent online backup
sqlite3 /var/lib/velora/velora.db \\
  ".backup /var/lib/velora/online-backup.db"`}
            </pre>
          </div>
        </div>
      </div>
    </div>
  );
}
