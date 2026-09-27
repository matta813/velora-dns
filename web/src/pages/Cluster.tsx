import { useCallback, useEffect, useState } from "react";
import {
  Server,
  RefreshCw,
  Clock,
  Hash,
  ShieldCheck,
  Info,
  History,
} from "lucide-react";
import {
  type ClusterNode,
  type ConfigVersion,
  loadClusterNodes,
  loadConfigVersions,
} from "../api-cluster";
import { useI18n } from "../i18n-context";
import { Stat } from "../components/Stat";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";

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
  }, [t]);

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
  }, [t]);

  useEffect(() => {
    const interval = window.setInterval(() => void refresh(), 30_000);
    return () => window.clearInterval(interval);
  }, [refresh]);

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
      <section className="panel">
        <Loading>{t("cluster.loading")}</Loading>
      </section>
    );
  }

  const healthyNodes = nodes.filter((n) => n.status === "healthy");

  return (
    <div className="stack">
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}

      <div className="notice info" role="status">
        <Info size={16} />
        {t("cluster.preview_notice")}
      </div>

      <div className="stats">
        <Stat label={t("cluster.total_nodes")} value={String(nodes.length)} icon={<Server size={17} />} />
        <Stat
          label={t("cluster.healthy")}
          value={`${healthyNodes.length} / ${nodes.length}`}
          icon={<ShieldCheck size={17} />}
          tone={healthyNodes.length < nodes.length ? "warning" : "brand"}
        />
        <Stat label={t("cluster.config_versions")} value={String(versions.length)} icon={<Hash size={17} />} tone="info" />
      </div>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              <Server size={17} /> {t("cluster.nodes")}
            </h2>
          </div>
          <button className="icon-button" aria-label={t("cluster.refresh")} title={t("cluster.refresh")} onClick={() => void refresh()}>
            <RefreshCw size={16} />
          </button>
        </div>
        {nodes.length === 0 ? (
          <EmptyState compact icon={<Server size={22} />} title={t("cluster.no_nodes")} />
        ) : (
          <div className="table-wrap">
            <table>
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
                      <small className="mono">{node.id.slice(0, 8)}</small>
                    </td>
                    <td><code>{node.address}</code></td>
                    <td>
                      <Badge tone={node.status === "healthy" ? "success" : "danger"}>{node.status}</Badge>
                    </td>
                    <td className="mono">{node.version || "—"}</td>
                    <td className="cell-muted">
                      <span className="endpoint">
                        <Clock size={13} /> {timeAgo(node.last_seen_at)}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              <History size={17} /> {t("cluster.config_history")}
            </h2>
          </div>
        </div>
        {versions.length === 0 ? (
          <EmptyState compact title={t("cluster.no_versions")} />
        ) : (
          <div className="table-wrap">
            <table>
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
                      <Badge tone="brand" plain>v{v.version}</Badge>
                    </td>
                    <td>
                      <code>{v.config_hash.slice(0, 12)}</code>
                    </td>
                    <td>{v.applied_by}</td>
                    <td className="cell-muted">{timeAgo(v.applied_at)}</td>
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
