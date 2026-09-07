export interface Status {
  ready: boolean;
  uptime_seconds: number;
  dns_listen: string[];
  version: { version: string; commit: string; built: string };
  capabilities: string[];
}
export interface Stats {
  queries_total: number;
  blocked_queries: number;
  queries_per_second: number;
  cache_hit_rate: number;
}
export interface Cache {
  entries: number;
  capacity: number;
  hits: number;
  misses: number;
}
export interface Config {
  query_log: {
    enabled: boolean;
    retention: number;
    max_rows: number;
    queue_size: number;
  };
  dns: {
    listen: string[];
    upstreams: string[];
    allowed_clients: string[];
    timeout: number;
    retries: number;
    max_concurrent: number;
  };
  cache: { max_entries: number };
  http: { listen: string; web_dir: string };
  log_level: string;
}
export interface Snapshot {
  status: Status;
  stats: Stats;
  cache: Cache;
  config: Config;
  checked: Date;
}
export interface QueryLogEntry {
  id: number;
  upstream: string;
  cache_hit: boolean;
  occurred_at: string;
  client_ip: string;
  domain: string;
  type: string;
  rcode: string;
  duration: number;
  source: string;
}
export interface BlocklistSource {
  id: number;
  name: string;
  url: string;
  enabled: boolean;
  last_updated_at?: string;
  last_error: string;
}
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}
export async function request<T>(
  path: string,
  signal?: AbortSignal,
  method = "GET",
  options: { body?: unknown; revision?: number } = {},
): Promise<T> {
  const response = await fetch(path, {
    method,
    signal,
    headers: {
      "Content-Type": "application/json",
      ...(options.revision === undefined
        ? {}
        : { "If-Match": `"${options.revision}"` }),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  if (!response.ok) {
    let message = `Management API returned HTTP ${response.status}`;
    try {
      const body: { error?: { message?: string } } = await response.json();
      if (typeof body.error?.message === "string") message = body.error.message;
    } catch {
      /* Proxies may return non-JSON error pages. */
    }
    throw new APIError(response.status, message);
  }
  const body: { data: T } = await response.json();
  return body.data;
}
export async function loadSnapshot(signal: AbortSignal): Promise<Snapshot> {
  const [status, stats, cache, config] = await Promise.all([
    request<Status>("/api/v1/status", signal),
    request<Stats>("/api/v1/stats", signal),
    request<Cache>("/api/v1/cache", signal),
    request<Config>("/api/v1/config", signal),
  ]);
  return { status, stats, cache, config, checked: new Date() };
}
