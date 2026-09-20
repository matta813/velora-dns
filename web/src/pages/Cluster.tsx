import { useCallback, useEffect, useState } from "react";
import {
  Server,
  RefreshCw,
  Clock,
  Hash,
  ShieldCheck,
  AlertTriangle,
} from "lucide-react";
import {
  type ClusterNode,
  type ConfigVersion,
  loadClusterNodes,
  loadConfigVersions,
} from "../api-cluster";
import { useI18n } from "../i18n-context";

export function Cluster() {
  const { t } = useI18n();
  const [nodes, setNodes] = useState<ClusterNode[]>([]);
  const [versions, setVersions] = useState<ConfigVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      const [n, v] = await Promise.all([
        loadClusterNodes(),
        loadConfigVersions(20),
      ]);
      setNodes(n);
      setVersions(v);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("cluster.load_failed"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let active = true;
    Promise.all([loadClusterNodes(), loadConfigVersions(20)])
      .then(([n, v]) => {
        if (active) {
          setNodes(n);
          setVersions(v);
          setError("");
        }
      })
      .catch((e) => {
        if (active) {
          setError(e instanceof Error ? e.message : t("cluster.load_failed"));
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  function timeAgo(dateStr: string): string {
    const d = new Date(dateStr);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return t("cluster.just_now");
    if (mins < 60) return `${mins}m`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ${mins % 60}m`;
    return `${Math.floor(hours / 24)}d`;
  }

  if (loading) {
    return (
      <section className="panel padded">
        <p>{t("cluster.loading")}</p>
      </section>
    );
  }

  const healthyNodes = nodes.filter((n) => n.status === "healthy");

  return (
    <>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}

      <div className="stats">
        <div className="stat">
          <div className="stat-icon">
            <Server size={17} />
          </div>
          <div>
            <span className="stat-label">{t("cluster.total_nodes")}</span>
            <strong className="stat-value">{nodes.length}</strong>
          </div>
        </div>
        <div className="stat">
          <div className="stat-icon">
            <ShieldCheck size={17} />
          </div>
          <div>
            <span className="stat-label">{t("cluster.healthy")}</span>
            <strong className="stat-value">{healthyNodes.length}</strong>
          </div>
        </div>
        <div className="stat">
          <div className="stat-icon">
            <Hash size={17} />
          </div>
          <div>
            <span className="stat-label">{t("cluster.config_versions")}</span>
            <strong className="stat-value">{versions.length}</strong>
          </div>
        </div>
      </div>

      <section className="panel padded">
        <div className="panel-header">
          <h2>
            <Server size={18} /> {t("cluster.nodes")}
          </h2>
          <button className="button secondary" onClick={() => void refresh()}>
            <RefreshCw size={15} />
          </button>
        </div>
        {nodes.length === 0 ? (
          <p className="muted">{t("cluster.no_nodes")}</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>{t("cluster.name")}</th>
                <th>{t("cluster.address")}</th>
                <th>{t("cluster.status")}</th>
                <th>{t("cluster.version")}</th>
                <th>{t("cluster.last_seen")}</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.id}>
                  <td>
                    <strong>{node.name}</strong>
                    <small>{node.id.slice(0, 8)}</small>
                  </td>
                  <td>{node.address}</td>
                  <td>
                    <span
                      className={`badge ${
                        node.status === "healthy" ? "badge-green" : "badge-red"
                      }`}
                    >
                      {node.status === "healthy" ? (
                        <ShieldCheck size={12} />
                      ) : (
                        <AlertTriangle size={12} />
                      )}
                      {node.status}
                    </span>
                  </td>
                  <td>{node.version || "-"}</td>
                  <td>
                    <Clock size={12} /> {timeAgo(node.last_seen_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel padded">
        <h2>{t("cluster.config_history")}</h2>
        {versions.length === 0 ? (
          <p className="muted">{t("cluster.no_versions")}</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>{t("cluster.version")}</th>
                <th>{t("cluster.hash")}</th>
                <th>{t("cluster.applied_by")}</th>
                <th>{t("cluster.applied_at")}</th>
              </tr>
            </thead>
            <tbody>
              {versions.map((v) => (
                <tr key={v.version}>
                  <td>
                    <strong>v{v.version}</strong>
                  </td>
                  <td>
                    <code>{v.config_hash.slice(0, 12)}</code>
                  </td>
                  <td>{v.applied_by}</td>
                  <td>{timeAgo(v.applied_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
