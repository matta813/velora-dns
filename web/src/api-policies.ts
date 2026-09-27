import { request } from "./api";
import type { Client } from "./api-clients";

export type PolicyMode = "default" | "disabled" | "custom";

export interface Policy {
  id: number;
  client_id: number;
  mode: PolicyMode;
  blocklists: number[];
  allow: string[];
  block: string[];
  enabled: boolean;
}

export type PolicyInput = Omit<Policy, "id">;

export interface EffectivePolicy {
  client: Client | null;
  policy: Policy | null;
  mode: PolicyMode;
}

export const loadPolicies = (signal?: AbortSignal) => request<Policy[]>("/api/v1/policies", signal);
export const createPolicy = (input: PolicyInput) =>
  request<Policy>("/api/v1/policies", undefined, "POST", { body: input });
export const updatePolicy = (id: number, input: PolicyInput) =>
  request<Policy>(`/api/v1/policies/${id}`, undefined, "PUT", { body: input });
export const deletePolicy = (id: number) =>
  request<{ deleted: number }>(`/api/v1/policies/${id}`, undefined, "DELETE");
export const effectivePolicy = (ip: string) =>
  request<EffectivePolicy>(`/api/v1/policies/effective?ip=${encodeURIComponent(ip)}`);
