package database

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/dhcp"
)

// Pool operations

func (s *Store) CreatePool(ctx context.Context, pool *dhcp.Pool) error {
	p := s.placeholder
	sql := fmt.Sprintf("INSERT INTO dhcp_pools(name, interface, subnet, gateway, dns_servers, lease_seconds, enabled) VALUES(%s,%s,%s,%s,%s,%s,%s)%s",
		p(1), p(2), p(3), p(4), p(5), p(6), p(7), s.insertReturning())
	dnsStr := encodeDNServers(pool.DNSServers)
	enabled := 0
	if pool.Enabled {
		enabled = 1
	}
	if s.driver == "postgres" {
		var id int64
		err := s.db.QueryRowContext(ctx, sql, pool.Name, pool.Interface, pool.Subnet.String(), pool.Gateway.String(), dnsStr, pool.LeaseSeconds, enabled).Scan(&id)
		if err != nil {
			return err
		}
		pool.ID = id
	} else {
		result, err := s.db.ExecContext(ctx, sql, pool.Name, pool.Interface, pool.Subnet.String(), pool.Gateway.String(), dnsStr, pool.LeaseSeconds, enabled)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		pool.ID = id
	}
	pool.CreatedAt = time.Now()
	pool.UpdatedAt = time.Now()
	return nil
}

func (s *Store) GetPool(ctx context.Context, id int64) (*dhcp.Pool, error) {
	p := s.placeholder
	row := s.db.QueryRowContext(ctx, "SELECT id, name, interface, subnet, gateway, dns_servers, lease_seconds, enabled, created_at, updated_at FROM dhcp_pools WHERE id="+p(1), id)
	return scanPool(row)
}

func (s *Store) ListPools(ctx context.Context) ([]dhcp.Pool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, interface, subnet, gateway, dns_servers, lease_seconds, enabled, created_at, updated_at FROM dhcp_pools ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []dhcp.Pool
	for rows.Next() {
		pool, err := scanPoolRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *pool)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePool(ctx context.Context, pool *dhcp.Pool) error {
	p := s.placeholder
	dnsStr := encodeDNServers(pool.DNSServers)
	enabled := 0
	if pool.Enabled {
		enabled = 1
	}
	sql := fmt.Sprintf("UPDATE dhcp_pools SET name=%s, interface=%s, subnet=%s, gateway=%s, dns_servers=%s, lease_seconds=%s, enabled=%s, updated_at=CURRENT_TIMESTAMP WHERE id=%s",
		p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8))
	_, err := s.db.ExecContext(ctx, sql, pool.Name, pool.Interface, pool.Subnet.String(), pool.Gateway.String(), dnsStr, pool.LeaseSeconds, enabled, pool.ID)
	return err
}

func (s *Store) DeletePool(ctx context.Context, id int64) error {
	p := s.placeholder
	_, err := s.db.ExecContext(ctx, "DELETE FROM dhcp_pools WHERE id="+p(1), id)
	return err
}

// Reservation operations

func (s *Store) CreateReservation(ctx context.Context, r *dhcp.Reservation) error {
	p := s.placeholder
	sql := fmt.Sprintf("INSERT INTO dhcp_reservations(pool_id, mac_address, ip_address, hostname) VALUES(%s,%s,%s,%s)%s",
		p(1), p(2), p(3), p(4), s.insertReturning())
	if s.driver == "postgres" {
		var id int64
		err := s.db.QueryRowContext(ctx, sql, r.PoolID, r.MACAddress, r.IPAddress, r.Hostname).Scan(&id)
		if err != nil {
			return err
		}
		r.ID = id
	} else {
		result, err := s.db.ExecContext(ctx, sql, r.PoolID, r.MACAddress, r.IPAddress, r.Hostname)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		r.ID = id
	}
	return nil
}

func (s *Store) ListReservations(ctx context.Context, poolID int64) ([]dhcp.Reservation, error) {
	p := s.placeholder
	rows, err := s.db.QueryContext(ctx, "SELECT id, pool_id, mac_address, ip_address, hostname FROM dhcp_reservations WHERE pool_id="+p(1)+" ORDER BY ip_address", poolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []dhcp.Reservation
	for rows.Next() {
		var r dhcp.Reservation
		if err := rows.Scan(&r.ID, &r.PoolID, &r.MACAddress, &r.IPAddress, &r.Hostname); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteReservation(ctx context.Context, id int64) error {
	p := s.placeholder
	_, err := s.db.ExecContext(ctx, "DELETE FROM dhcp_reservations WHERE id="+p(1), id)
	return err
}

// Lease operations

func (s *Store) SaveLease(ctx context.Context, l *dhcp.Lease) error {
	p := s.placeholder
	sql := fmt.Sprintf("INSERT INTO dhcp_leases(pool_id, mac_address, ip_address, hostname, client_id, expires_at, status) VALUES(%s,%s,%s,%s,%s,%s,%s) ON CONFLICT(pool_id, ip_address) DO UPDATE SET mac_address=excluded.mac_address, hostname=excluded.hostname, client_id=excluded.client_id, expires_at=excluded.expires_at, status=excluded.status",
		p(1), p(2), p(3), p(4), p(5), p(6), p(7))
	_, err := s.db.ExecContext(ctx, sql, l.PoolID, l.MACAddress, l.IPAddress, l.Hostname, l.ClientID, l.ExpiresAt.Format(time.RFC3339), string(l.Status))
	return err
}

func (s *Store) GetLease(ctx context.Context, poolID int64, mac string) (*dhcp.Lease, error) {
	p := s.placeholder
	row := s.db.QueryRowContext(ctx, "SELECT id, pool_id, mac_address, ip_address, hostname, client_id, expires_at, status, created_at FROM dhcp_leases WHERE pool_id="+p(1)+" AND mac_address="+p(2), poolID, mac)
	return scanLease(row)
}

func (s *Store) GetLeaseByIP(ctx context.Context, poolID int64, ip netip.Addr) (*dhcp.Lease, error) {
	p := s.placeholder
	row := s.db.QueryRowContext(ctx, "SELECT id, pool_id, mac_address, ip_address, hostname, client_id, expires_at, status, created_at FROM dhcp_leases WHERE pool_id="+p(1)+" AND ip_address="+p(2), poolID, ip.String())
	return scanLease(row)
}

func (s *Store) ListLeases(ctx context.Context, poolID int64) ([]dhcp.Lease, error) {
	p := s.placeholder
	rows, err := s.db.QueryContext(ctx, "SELECT id, pool_id, mac_address, ip_address, hostname, client_id, expires_at, status, created_at FROM dhcp_leases WHERE pool_id="+p(1)+" ORDER BY ip_address", poolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLeases(rows)
}

func (s *Store) ListActiveLeases(ctx context.Context) ([]dhcp.Lease, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, pool_id, mac_address, ip_address, hostname, client_id, expires_at, status, created_at FROM dhcp_leases WHERE status='active' ORDER BY ip_address")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLeases(rows)
}

func (s *Store) ExpireLeases(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, "UPDATE dhcp_leases SET status='expired' WHERE status='active' AND expires_at < datetime('now')")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) DeleteLease(ctx context.Context, id int64) error {
	p := s.placeholder
	_, err := s.db.ExecContext(ctx, "DELETE FROM dhcp_leases WHERE id="+p(1), id)
	return err
}

// Pool implements dhcp.Store on *Store.
var _ dhcp.Store = (*Store)(nil)

// scanPool helpers

type scannable interface {
	Scan(dest ...any) error
}

func scanPool(row scannable) (*dhcp.Pool, error) {
	var pool dhcp.Pool
	var subnet, gateway, dnsStr string
	var enabled int
	if err := row.Scan(&pool.ID, &pool.Name, &pool.Interface, &subnet, &gateway, &dnsStr, &pool.LeaseSeconds, &enabled, &pool.CreatedAt, &pool.UpdatedAt); err != nil {
		return nil, err
	}
	pool.Subnet, _ = netip.ParsePrefix(subnet)
	pool.Gateway, _ = netip.ParseAddr(gateway)
	pool.DNSServers = decodeDNServers(dnsStr)
	pool.Enabled = enabled == 1
	return &pool, nil
}

func scanPoolRows(rows interface{ Scan(dest ...any) error }) (*dhcp.Pool, error) {
	return scanPool(rows)
}

func scanLease(row scannable) (*dhcp.Lease, error) {
	var l dhcp.Lease
	var expiresAt string
	var status string
	if err := row.Scan(&l.ID, &l.PoolID, &l.MACAddress, &l.IPAddress, &l.Hostname, &l.ClientID, &expiresAt, &status, &l.CreatedAt); err != nil {
		return nil, err
	}
	l.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	l.Status = dhcp.LeaseStatus(status)
	return &l, nil
}

func scanLeases(rows interface {
	Scan(dest ...any) error
	Next() bool
	Err() error
}) ([]dhcp.Lease, error) {
	var out []dhcp.Lease
	for rows.Next() {
		var l dhcp.Lease
		var expiresAt string
		var status string
		if err := rows.Scan(&l.ID, &l.PoolID, &l.MACAddress, &l.IPAddress, &l.Hostname, &l.ClientID, &expiresAt, &status, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
		l.Status = dhcp.LeaseStatus(status)
		out = append(out, l)
	}
	return out, rows.Err()
}

func encodeDNServers(addrs []netip.Addr) string {
	if len(addrs) == 0 {
		return ""
	}
	var parts []string
	for _, a := range addrs {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ",")
}

func decodeDNServers(s string) []netip.Addr {
	if s == "" {
		return nil
	}
	var out []netip.Addr
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if addr, err := netip.ParseAddr(part); err == nil {
			out = append(out, addr)
		}
	}
	return out
}
