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
