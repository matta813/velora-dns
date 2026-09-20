CREATE TABLE dhcp_pools (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    interface TEXT NOT NULL,
    subnet TEXT NOT NULL,
    gateway TEXT NOT NULL,
    dns_servers TEXT NOT NULL DEFAULT '',
    lease_seconds INTEGER NOT NULL DEFAULT 86400,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE dhcp_reservations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pool_id INTEGER NOT NULL REFERENCES dhcp_pools(id) ON DELETE CASCADE,
    mac_address TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    UNIQUE(pool_id, mac_address),
    UNIQUE(pool_id, ip_address)
);

CREATE TABLE dhcp_leases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pool_id INTEGER NOT NULL REFERENCES dhcp_pools(id) ON DELETE CASCADE,
    mac_address TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'expired', 'released')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(pool_id, ip_address)
);

CREATE INDEX idx_dhcp_leases_pool_status ON dhcp_leases(pool_id, status);
CREATE INDEX idx_dhcp_leases_mac ON dhcp_leases(mac_address);
