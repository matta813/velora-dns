import { request } from "./api";

export interface DHCPPool {
  id: number;
  name: string;
  interface: string;
  subnet: string;
  gateway: string;
  dns_servers: string[];
  lease_seconds: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface DHCPReservation {
  id: number;
  pool_id: number;
  mac_address: string;
  ip_address: string;
  hostname: string;
}

export interface DHCPLease {
  id: number;
  pool_id: number;
  mac_address: string;
  ip_address: string;
  hostname: string;
  client_id: string;
  expires_at: string;
  status: string;
  created_at: string;
}

export async function loadPools(signal?: AbortSignal): Promise<DHCPPool[]> {
  return request("/api/v1/dhcp/pools", signal);
}

export async function createPool(
  pool: Omit<DHCPPool, "id" | "created_at" | "updated_at">,
): Promise<DHCPPool> {
  return request("/api/v1/dhcp/pools", undefined, "POST", { body: pool });
}

export async function deletePool(id: number): Promise<void> {
  return request(`/api/v1/dhcp/pools/${id}`, undefined, "DELETE");
}

export async function loadReservations(
  poolId: number,
  signal?: AbortSignal,
): Promise<DHCPReservation[]> {
  return request(`/api/v1/dhcp/pools/${poolId}/reservations`, signal);
}

export async function createReservation(
  poolId: number,
  r: Omit<DHCPReservation, "id" | "pool_id">,
): Promise<DHCPReservation> {
  return request(`/api/v1/dhcp/pools/${poolId}/reservations`, undefined, "POST", {
    body: r,
  });
}

export async function deleteReservation(id: number): Promise<void> {
  return request(`/api/v1/dhcp/reservations/${id}`, undefined, "DELETE");
}

export async function loadLeases(signal?: AbortSignal): Promise<DHCPLease[]> {
  return request("/api/v1/dhcp/leases", signal);
}

export async function deleteLease(id: number): Promise<void> {
  return request(`/api/v1/dhcp/leases/${id}`, undefined, "DELETE");
}
