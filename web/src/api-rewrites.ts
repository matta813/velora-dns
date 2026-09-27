import { request } from "./api";

export type RewriteType = "A" | "AAAA" | "CNAME";

export interface Rewrite {
  id: number;
  name: string;
  type: RewriteType;
  value: string;
  enabled: boolean;
  description: string;
}

export interface RewriteStatus extends Rewrite {
  blocked_by?: string;
  overrides_zone?: string;
}

export type RewriteInput = Omit<Rewrite, "id">;

export const loadRewrites = (signal?: AbortSignal) => request<RewriteStatus[]>("/api/v1/rewrites", signal);
export const createRewrite = (input: RewriteInput) =>
  request<Rewrite>("/api/v1/rewrites", undefined, "POST", { body: input });
export const updateRewrite = (id: number, input: RewriteInput) =>
  request<Rewrite>(`/api/v1/rewrites/${id}`, undefined, "PUT", { body: input });
export const deleteRewrite = (id: number) =>
  request<{ deleted: number }>(`/api/v1/rewrites/${id}`, undefined, "DELETE");
