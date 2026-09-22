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
export interface ClusterState { configured: boolean; cluster: { cluster_id: string; node_id: string; node_name: string; control_address: string; role: string } }
export interface JoinBundle { leader_address: string; ca_certificate: string; token: string; expires_at: string }
export async function loadClusterState(): Promise<ClusterState> { return request<ClusterState>("/api/v1/cluster"); }
export async function createCluster(name: string, control_address: string) { return request<ClusterState["cluster"]>("/api/v1/cluster", undefined, "POST", { body: { name, control_address } }); }
export async function createClusterJoinToken(): Promise<JoinBundle> { return request<JoinBundle>("/api/v1/cluster/join-tokens", undefined, "POST"); }
export async function joinCluster(name: string, control_address: string, bundle: JoinBundle) { return request<ClusterState["cluster"]>("/api/v1/cluster/join", undefined, "POST", { body: { name, control_address, bundle } }); }

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
