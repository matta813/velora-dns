import { request } from "./api";

export const EXPLAIN_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "PTR", "SOA"] as const;

export interface ExplainRule {
  id?: number;
  name: string;
  detail?: string;
}
export interface ExplainFilter {
  scope: "global" | "client_policy";
  client?: string;
  mode?: string;
  matches?: ExplainRule[];
  allowed_by?: ExplainRule[];
}
export interface ExplainStep {
  stage: "filter" | "rewrite" | "zone" | "cache" | "forwarding" | "upstream" | "refused";
  result: string;
  rules?: ExplainRule[];
  filter?: ExplainFilter;
  remaining_ttl?: number;
  depth: number;
}
export interface AnswerExplanation {
  name: string;
  type: string;
  client?: string;
  source: "local" | "cache" | "blocked" | "upstream" | "refused";
  winner: ExplainStep;
  rcode?: string;
  answers: string[];
  steps: ExplainStep[];
}

export const explainAnswer = (input: { name: string; type: string; client?: string }, signal?: AbortSignal) =>
  request<AnswerExplanation>("/api/v1/diagnostics/explain", signal, "POST", { body: input });
