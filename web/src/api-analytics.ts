import { request, type QueryRanking } from "./api";

export type AnalyticsRange = "1h" | "24h" | "7d" | "30d";

export interface AnalyticsBucket {
  start: string;
  total: number;
  blocked: number;
  cached: number;
  failed: number;
  average_ms: number;
}

export interface UpstreamUsage {
  address: string;
  queries: number;
  failed: number;
  average_ms: number;
}

export interface Analytics {
  range: AnalyticsRange;
  window_start: string;
  window_end: string;
  bucket_seconds: number;
  history_start: string | null;
  totals: AnalyticsBucket;
  series: AnalyticsBucket[];
  query_types: QueryRanking[];
  response_codes: QueryRanking[];
  sources: QueryRanking[];
  upstreams: UpstreamUsage[];
  top_blocked: QueryRanking[];
}

export const loadAnalytics = (range: AnalyticsRange, signal?: AbortSignal) =>
  request<Analytics>(`/api/v1/analytics?range=${range}`, signal);
