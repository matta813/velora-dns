import { useCallback, useEffect, useState } from "react";
import { Network, Plus, Trash2, RefreshCw } from "lucide-react";
import {
  type DHCPPool,
  type DHCPLease,
  type DHCPReservation,
  loadPools,
  loadLeases,
  loadReservations,
  createPool,
  createReservation,
  deletePool,
  deleteReservation,
  deleteLease,
} from "../api-dhcp";
import { useI18n } from "../i18n-context";
import { EmptyState, Loading } from "../components/EmptyState";

export function DHCP({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
  const [pools, setPools] = useState<DHCPPool[]>([]);
  const [leases, setLeases] = useState<DHCPLease[]>([]);
  const [reservations, setReservations] = useState<DHCPReservation[]>([]);
  const [selectedPool, setSelectedPool] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showCreatePool, setShowCreatePool] = useState(false);
  const [newPool, setNewPool] = useState({
    name: "",
    interface: "eth0",
    subnet: "",
    gateway: "",
    dns_servers: ["1.1.1.1"],
    lease_seconds: 86400,
  });
  const [newReservation, setNewReservation] = useState({
    mac_address: "",
    ip_address: "",
    hostname: "",
  });

  const refresh = useCallback(async () => {
    try {
      const [p, l] = await Promise.all([loadPools(), loadLeases()]);
      setPools(p);
      setLeases(l);
      if (selectedPool) {
        setReservations(await loadReservations(selectedPool));
      }
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.load_failed"));
    } finally {
      setLoading(false);
    }
  }, [selectedPool, t]);

  useEffect(() => {
    let active = true;
    Promise.all([loadPools(), loadLeases()])
      .then(([p, l]) => {
        if (active) {
          setPools(p);
          setLeases(l);
          setError("");
        }
      })
      .catch((e) => {
        if (active) {
          setError(e instanceof Error ? e.message : t("dhcp.load_failed"));
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [t]);

  async function handleCreatePool() {
    try {
      await createPool({
        ...newPool,
        enabled: true,
      });
      setShowCreatePool(false);
      setNewPool({
        name: "",
        interface: "eth0",
        subnet: "",
        gateway: "",
        dns_servers: ["1.1.1.1"],
        lease_seconds: 86400,
      });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.create_pool_failed"));
    }
  }

  async function handleDeletePool(id: number) {
    if (!confirm(t("dhcp.confirm_delete_pool"))) return;
    try {
      await deletePool(id);
      if (selectedPool === id) setSelectedPool(null);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.delete_pool_failed"));
    }
  }

  async function handleCreateReservation() {
    if (!selectedPool) return;
    try {
      await createReservation(selectedPool, newReservation);
      setNewReservation({ mac_address: "", ip_address: "", hostname: "" });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.create_reservation_failed"));
    }
  }

  async function handleDeleteReservation(id: number) {
    try {
      await deleteReservation(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.delete_reservation_failed"));
    }
  }

  async function handleDeleteLease(id: number) {
    try {
      await deleteLease(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("dhcp.delete_lease_failed"));
    }
  }

  function formatExpiry(expiresAt: string): string {
    const d = new Date(expiresAt);
    const now = new Date();
    const diff = d.getTime() - now.getTime();
    if (diff <= 0) return t("dhcp.expired");
    const hours = Math.floor(diff / 3600000);
    const mins = Math.floor((diff % 3600000) / 60000);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
    return `${hours}h ${mins}m`;
  }

  if (loading) {
    return (
      <section className="panel">
        <Loading>{t("dhcp.loading")}</Loading>
      </section>
    );
  }

  const activeLeases = leases.filter((l) => l.status === "active");
  const pool = pools.find((p) => p.id === selectedPool);

  return (
    <div className="stack">
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              {t("dhcp.pools")}
            </h2>
          </div>
          <div className="button-group">
            <button className="icon-button" onClick={() => void refresh()} aria-label={t("app.refresh")} title={t("app.refresh")}>
              <RefreshCw size={16} />
            </button>
            {!readOnly && (
              <button className="button primary" onClick={() => setShowCreatePool(!showCreatePool)}>
                <Plus size={15} />
                {t("dhcp.add_pool")}
              </button>
            )}
          </div>
        </div>

        {showCreatePool && (
          <form
            className="zone-form"
            onSubmit={(e) => {
              e.preventDefault();
              void handleCreatePool();
            }}
          >
            <div className="form-grid">
              <label>
                {t("dhcp.pool_name")}
                <input value={newPool.name} onChange={(e) => setNewPool({ ...newPool, name: e.target.value })} placeholder="lan" />
              </label>
              <label>
                {t("dhcp.interface")}
                <input value={newPool.interface} onChange={(e) => setNewPool({ ...newPool, interface: e.target.value })} />
              </label>
              <label>
                {t("dhcp.subnet_cidr")}
                <input value={newPool.subnet} onChange={(e) => setNewPool({ ...newPool, subnet: e.target.value })} placeholder="192.168.1.0/24" />
              </label>
              <label>
                {t("dhcp.gateway")}
                <input value={newPool.gateway} onChange={(e) => setNewPool({ ...newPool, gateway: e.target.value })} placeholder="192.168.1.1" />
              </label>
              <label>
                {t("dhcp.dns_servers")}
                <input
                  value={newPool.dns_servers.join(", ")}
                  onChange={(e) => setNewPool({ ...newPool, dns_servers: e.target.value.split(",").map((s) => s.trim()) })}
                  placeholder="1.1.1.1, 8.8.8.8"
                />
              </label>
              <label>
                {t("dhcp.lease_seconds")}
                <input
                  type="number"
                  value={newPool.lease_seconds}
                  onChange={(e) => setNewPool({ ...newPool, lease_seconds: Number(e.target.value) })}
                />
              </label>
            </div>
            <div className="form-actions">
              <button className="button primary" type="submit">
                {t("dhcp.create")}
              </button>
              <button className="button" type="button" onClick={() => setShowCreatePool(false)}>
                {t("zones.cancel")}
              </button>
            </div>
          </form>
        )}

        {pools.length === 0 ? (
          <EmptyState compact icon={<Network size={22} />} title={t("dhcp.no_pools")} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("dhcp.name")}</th>
                  <th>{t("dhcp.subnet")}</th>
                  <th>{t("dhcp.gateway")}</th>
                  <th className="num">{t("dhcp.leases_label")}</th>
                  <th>
                    <span className="sr-only">{t("zones.col_actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {pools.map((pool) => {
                  const poolLeases = activeLeases.filter((l) => l.pool_id === pool.id);
                  return (
                    <tr
                      key={pool.id}
                      className={`clickable${selectedPool === pool.id ? " selected" : ""}`}
                      onClick={() => setSelectedPool(pool.id)}
                      tabIndex={0}
                      onKeyDown={(e) => {
                        if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) {
                          e.preventDefault();
                          setSelectedPool(pool.id);
                        }
                      }}
                    >
                      <td>
                        <strong>{pool.name}</strong>
                        <small>{pool.interface}</small>
                      </td>
                      <td><code>{pool.subnet}</code></td>
                      <td><code>{pool.gateway}</code></td>
                      <td className="num mono">{poolLeases.length}</td>
                      <td>
                        <div className="table-actions">
                          {!readOnly && (
                            <button
                              className="icon-button danger-icon"
                              aria-label={`${t("zones.delete_aria")} ${pool.name}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                void handleDeletePool(pool.id);
                              }}
                            >
                              <Trash2 size={15} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {selectedPool && (
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("dhcp.reservations")}</h2>
              {pool && <p>{pool.name} · {pool.subnet}</p>}
            </div>
          </div>
          {!readOnly && (
            <form
              className="zone-form"
              onSubmit={(e) => {
                e.preventDefault();
                void handleCreateReservation();
              }}
            >
              <div className="form-grid">
                <label>
                  {t("dhcp.mac_address")}
                  <input
                    value={newReservation.mac_address}
                    onChange={(e) => setNewReservation({ ...newReservation, mac_address: e.target.value })}
                    placeholder="aa:bb:cc:dd:ee:ff"
                  />
                </label>
                <label>
                  {t("dhcp.ip_address")}
                  <input
                    value={newReservation.ip_address}
                    onChange={(e) => setNewReservation({ ...newReservation, ip_address: e.target.value })}
                    placeholder="192.168.1.100"
                  />
                </label>
                <label>
                  {t("dhcp.hostname")}
                  <input
                    value={newReservation.hostname}
                    onChange={(e) => setNewReservation({ ...newReservation, hostname: e.target.value })}
                    placeholder="printer"
                  />
                </label>
              </div>
              <div className="form-actions">
                <button className="button primary" type="submit">
                  <Plus size={15} />
                  {t("dhcp.add")}
                </button>
              </div>
            </form>
          )}
          {reservations.length > 0 && (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t("dhcp.mac_address")}</th>
                    <th>{t("dhcp.ip_address")}</th>
                    <th>{t("dhcp.hostname")}</th>
                    <th>
                      <span className="sr-only">{t("zones.col_actions")}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {reservations.map((r) => (
                    <tr key={r.id}>
                      <td><code>{r.mac_address}</code></td>
                      <td><code>{r.ip_address}</code></td>
                      <td>{r.hostname || <span className="cell-muted">—</span>}</td>
                      <td>
                        <div className="table-actions">
                          {!readOnly && (
                            <button
                              className="icon-button danger-icon"
                              aria-label={`${t("zones.delete_aria")} ${r.mac_address}`}
                              onClick={() => void handleDeleteReservation(r.id)}
                            >
                              <Trash2 size={15} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}

      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              {t("dhcp.active_leases")}
            </h2>
          </div>
          <span className="subtle-badge">{activeLeases.length}</span>
        </div>
        {activeLeases.length === 0 ? (
          <EmptyState compact title={t("dhcp.no_leases")} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("dhcp.mac_address")}</th>
                  <th>{t("dhcp.ip_address")}</th>
                  <th>{t("dhcp.hostname")}</th>
                  <th>{t("dhcp.expires")}</th>
                  <th>
                    <span className="sr-only">{t("zones.col_actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {activeLeases.map((lease) => (
                  <tr key={lease.id}>
                    <td><code>{lease.mac_address}</code></td>
                    <td><code>{lease.ip_address}</code></td>
                    <td>{lease.hostname || <span className="cell-muted">—</span>}</td>
                    <td className="cell-muted">{formatExpiry(lease.expires_at)}</td>
                    <td>
                      <div className="table-actions">
                        {!readOnly && (
                          <button
                            className="icon-button danger-icon"
                            aria-label={`${t("zones.delete_aria")} ${lease.ip_address}`}
                            onClick={() => void handleDeleteLease(lease.id)}
                          >
                            <Trash2 size={15} />
                          </button>
                        )}
                      </div>
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
