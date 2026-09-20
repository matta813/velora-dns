import { useCallback, useEffect, useState } from "react";
import { Network, Plus, Trash2, RefreshCw, ServerCrash } from "lucide-react";
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
      setError(e instanceof Error ? e.message : "Failed to load DHCP data");
    } finally {
      setLoading(false);
    }
  }, [selectedPool]);

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
          setError(e instanceof Error ? e.message : "Failed to load DHCP data");
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

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
      setError(e instanceof Error ? e.message : "Failed to create pool");
    }
  }

  async function handleDeletePool(id: number) {
    if (!confirm(t("dhcp.confirm_delete_pool"))) return;
    try {
      await deletePool(id);
      if (selectedPool === id) setSelectedPool(null);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete pool");
    }
  }

  async function handleCreateReservation() {
    if (!selectedPool) return;
    try {
      await createReservation(selectedPool, newReservation);
      setNewReservation({ mac_address: "", ip_address: "", hostname: "" });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create reservation");
    }
  }

  async function handleDeleteReservation(id: number) {
    try {
      await deleteReservation(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete reservation");
    }
  }

  async function handleDeleteLease(id: number) {
    try {
      await deleteLease(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete lease");
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
      <section className="panel padded">
        <p>{t("dhcp.loading")}</p>
      </section>
    );
  }

  return (
    <>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}

      <section className="panel padded">
        <div className="panel-header">
          <h2>
            <Network size={18} /> {t("dhcp.pools")}
          </h2>
          <div className="panel-actions">
            <button className="button secondary" onClick={() => void refresh()}>
              <RefreshCw size={15} />
            </button>
            {!readOnly && (
              <button
                className="button"
                onClick={() => setShowCreatePool(!showCreatePool)}
              >
                <Plus size={15} />
                {t("dhcp.add_pool")}
              </button>
            )}
          </div>
        </div>

        {showCreatePool && (
          <div className="form-grid">
            <label>
              {t("dhcp.pool_name")}
              <input
                value={newPool.name}
                onChange={(e) => setNewPool({ ...newPool, name: e.target.value })}
                placeholder="lan"
              />
            </label>
            <label>
              {t("dhcp.interface")}
              <input
                value={newPool.interface}
                onChange={(e) =>
                  setNewPool({ ...newPool, interface: e.target.value })
                }
              />
            </label>
            <label>
              {t("dhcp.subnet_cidr")}
              <input
                value={newPool.subnet}
                onChange={(e) => setNewPool({ ...newPool, subnet: e.target.value })}
                placeholder="192.168.1.0/24"
              />
            </label>
            <label>
              {t("dhcp.gateway")}
              <input
                value={newPool.gateway}
                onChange={(e) =>
                  setNewPool({ ...newPool, gateway: e.target.value })
                }
                placeholder="192.168.1.1"
              />
            </label>
            <label>
              {t("dhcp.dns_servers")}
              <input
                value={newPool.dns_servers.join(", ")}
                onChange={(e) =>
                  setNewPool({
                    ...newPool,
                    dns_servers: e.target.value.split(",").map((s) => s.trim()),
                  })
                }
                placeholder="1.1.1.1, 8.8.8.8"
              />
            </label>
            <label>
              {t("dhcp.lease_seconds")}
              <input
                type="number"
                value={newPool.lease_seconds}
                onChange={(e) =>
                  setNewPool({
                    ...newPool,
                    lease_seconds: Number(e.target.value),
                  })
                }
              />
            </label>
            <button className="button" onClick={() => void handleCreatePool()}>
              {t("dhcp.create")}
            </button>
          </div>
        )}

        {pools.length === 0 ? (
          <p className="muted">{t("dhcp.no_pools")}</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>{t("dhcp.name")}</th>
                <th>{t("dhcp.subnet")}</th>
                <th>{t("dhcp.gateway")}</th>
                <th>{t("dhcp.leases_label")}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pools.map((pool) => {
                const poolLeases = leases.filter((l) => l.pool_id === pool.id && l.status === "active");
                return (
                  <tr
                    key={pool.id}
                    className={selectedPool === pool.id ? "selected" : ""}
                    onClick={() => setSelectedPool(pool.id)}
                    style={{ cursor: "pointer" }}
                  >
                    <td>
                      <strong>{pool.name}</strong>
                      <small>{pool.interface}</small>
                    </td>
                    <td>{pool.subnet}</td>
                    <td>{pool.gateway}</td>
                    <td>{poolLeases.length}</td>
                    <td>
                      {!readOnly && (
                        <button
                          className="button small danger"
                          onClick={(e) => {
                            e.stopPropagation();
                            void handleDeletePool(pool.id);
                          }}
                        >
                          <Trash2 size={14} />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      {selectedPool && (
        <section className="panel padded">
          <h3>{t("dhcp.reservations")}</h3>
          <div className="form-grid compact">
            <label>
              {t("dhcp.mac_address")}
              <input
                value={newReservation.mac_address}
                onChange={(e) =>
                  setNewReservation({
                    ...newReservation,
                    mac_address: e.target.value,
                  })
                }
                placeholder="aa:bb:cc:dd:ee:ff"
              />
            </label>
            <label>
              {t("dhcp.ip_address")}
              <input
                value={newReservation.ip_address}
                onChange={(e) =>
                  setNewReservation({
                    ...newReservation,
                    ip_address: e.target.value,
                  })
                }
                placeholder="192.168.1.100"
              />
            </label>
            <label>
              {t("dhcp.hostname")}
              <input
                value={newReservation.hostname}
                onChange={(e) =>
                  setNewReservation({
                    ...newReservation,
                    hostname: e.target.value,
                  })
                }
                placeholder="printer"
              />
            </label>
            {!readOnly && (
              <button
                className="button"
                onClick={() => void handleCreateReservation()}
              >
                <Plus size={15} />
                {t("dhcp.add")}
              </button>
            )}
          </div>
          {reservations.length > 0 && (
            <table className="table">
              <thead>
                <tr>
                  <th>{t("dhcp.mac_address")}</th>
                  <th>{t("dhcp.ip_address")}</th>
                  <th>{t("dhcp.hostname")}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {reservations.map((r) => (
                  <tr key={r.id}>
                    <td><code>{r.mac_address}</code></td>
                    <td>{r.ip_address}</td>
                    <td>{r.hostname || "-"}</td>
                    <td>
                      {!readOnly && (
                        <button
                          className="button small danger"
                          onClick={() => void handleDeleteReservation(r.id)}
                        >
                          <Trash2 size={14} />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      <section className="panel padded">
        <h3>
          <ServerCrash size={18} /> {t("dhcp.active_leases")}
        </h3>
        {leases.filter((l) => l.status === "active").length === 0 ? (
          <p className="muted">{t("dhcp.no_leases")}</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>{t("dhcp.mac_address")}</th>
                <th>{t("dhcp.ip_address")}</th>
                <th>{t("dhcp.hostname")}</th>
                <th>{t("dhcp.expires")}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {leases
                .filter((l) => l.status === "active")
                .map((lease) => (
                  <tr key={lease.id}>
                    <td><code>{lease.mac_address}</code></td>
                    <td>{lease.ip_address}</td>
                    <td>{lease.hostname || "-"}</td>
                    <td>{formatExpiry(lease.expires_at)}</td>
                    <td>
                      {!readOnly && (
                        <button
                          className="button small danger"
                          onClick={() => void handleDeleteLease(lease.id)}
                        >
                          <Trash2 size={14} />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
