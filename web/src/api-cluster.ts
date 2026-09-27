import { request } from "./api";

export interface ClusterNode {
  id: string;
  name: string;
  address: string;
  capabilities: string[];
  version: string;
  status: string;
  last_seen_at: string;
}

export interface ConfigVersion {
  version: number;
  config_hash: string;
  applied_by: string;
  applied_at: string;
}

export async function loadClusterNodes(signal?: AbortSignal): Promise<ClusterNode[]> {
  return (await request<ClusterNode[] | null>("/api/v1/cluster/nodes", signal)) ?? [];
}

export async function deleteClusterNode(id: string): Promise<void> {
  return request(`/api/v1/cluster/nodes/${id}`, undefined, "DELETE");
}

export async function loadConfigVersions(
  limit = 10,
  signal?: AbortSignal,
): Promise<ConfigVersion[]> {
  return (
    (await request<ConfigVersion[] | null>(
      `/api/v1/cluster/config-versions?limit=${limit}`,
      signal,
    )) ?? []
  );
}

export type ClusterRole = "standalone" | "primary" | "replica";
export type MemberStatus = "in_sync" | "behind" | "stale" | "error" | "never_synced";

export interface ClusterState {
  role: ClusterRole;
  cluster_id: string;
  node_id: string;
  node_name: string;
  advertised_url: string;
  primary_url: string;
  allow_insecure: boolean;
  created_at: string | null;
  last_sync_at: string | null;
  last_sync_error: string;
  applied_revision: string;
}

export interface ClusterMember {
  node_id: string;
  name: string;
  address: string;
  version: string;
  joined_at: string;
  last_seen_at: string | null;
  applied_revision: string;
  last_error: string;
  status: MemberStatus;
}

export interface ClusterOverview {
  state: ClusterState;
  revision: string;
  members: ClusterMember[];
  zones_read_only: boolean;
  protocol: number;
  version: string;
  replicated_zones: number;
}

export const loadClusterOverview = (signal?: AbortSignal) => request<ClusterOverview>("/api/v1/cluster/overview", signal);
export const createCluster = (body: { name: string; advertised_url: string; allow_insecure: boolean; skip_check: boolean }) =>
  request<ClusterOverview>("/api/v1/cluster/create", undefined, "POST", { body });
export const issueJoinToken = () => request<{ token: string; expires_at: string }>("/api/v1/cluster/join-tokens", undefined, "POST");
export const joinCluster = (body: { primary_url: string; token: string; name: string; allow_insecure: boolean }) =>
  request<ClusterOverview>("/api/v1/cluster/connect", undefined, "POST", { body });
export const syncCluster = () => request<ClusterOverview>("/api/v1/cluster/sync", undefined, "POST");
export const removeClusterMember = (id: string) => request<ClusterOverview>(`/api/v1/cluster/members/${encodeURIComponent(id)}`, undefined, "DELETE");
export const leaveCluster = () => request<ClusterOverview>("/api/v1/cluster/leave", undefined, "POST");
export const dissolveCluster = () => request<ClusterOverview>("/api/v1/cluster/dissolve", undefined, "POST");
