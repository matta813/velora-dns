import { request } from "./api";

export interface UpstreamStatus {
  address: string;
  state: "unknown" | "healthy" | "degraded" | "unavailable";
  consecutive_failures: number;
  latency_milliseconds: number;
}

export interface ForwardRule {
  id: number;
  domain: string;
  upstreams: string[];
  enabled: boolean;
  description: string;
}

export interface ForwardRuleStatus extends ForwardRule {
  health: UpstreamStatus[];
}

export interface ForwardRuleInput {
  domain: string;
  upstreams: string[];
  enabled: boolean;
  description: string;
}

export interface ForwardTestResult {
  name: string;
  type: string;
  rcode?: string;
  answers: string[];
  upstream?: string;
  duration_ms: number;
  error?: string;
}

export const loadForwardRules = (signal?: AbortSignal) =>
  request<ForwardRuleStatus[]>("/api/v1/forwarding", signal);

export const createForwardRule = (input: ForwardRuleInput) =>
  request<ForwardRule>("/api/v1/forwarding", undefined, "POST", { body: input });

export const updateForwardRule = (id: number, input: ForwardRuleInput) =>
  request<ForwardRule>(`/api/v1/forwarding/${id}`, undefined, "PUT", { body: input });

export const deleteForwardRule = (id: number) =>
  request<{ deleted: number }>(`/api/v1/forwarding/${id}`, undefined, "DELETE");

export const testForwardRule = (id: number, name: string, type: string) =>
  request<ForwardTestResult>(`/api/v1/forwarding/${id}/test`, undefined, "POST", { body: { name, type } });
