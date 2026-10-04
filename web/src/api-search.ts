import { request } from "./api";

export type SearchKind = "zone" | "record" | "client" | "rewrite" | "forwarding" | "blocklist";

export interface SearchResult {
  kind: SearchKind;
  title: string;
  subtitle?: string;
  link: string;
}

export const searchResources = (query: string, signal?: AbortSignal) =>
  request<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(query)}`, signal);
