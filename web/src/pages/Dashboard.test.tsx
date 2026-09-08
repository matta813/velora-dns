import { render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Dashboard } from "./Dashboard";
import type { Snapshot } from "../api";

const snapshot = {
  status: { ready: true, uptime_seconds: 60, dns_listen: [], version: { version: "dev", commit: "", built: "" }, capabilities: [] },
  stats: { queries_total: 3, blocked_queries: 1, queries_per_second: 0.1, cache_hit_rate: 0 },
  cache: { entries: 0, capacity: 100, hits: 0, misses: 0 },
  config: { query_log: { enabled: true, retention: 0, max_rows: 100, queue_size: 10 }, dns: { listen: [], upstreams: ["127.0.0.1:53"], allowed_clients: [], timeout: 2e9, retries: 0, max_concurrent: 1 }, cache: { max_entries: 100 }, http: { listen: "127.0.0.1:8080", web_dir: "web/dist" }, log_level: "info" },
  checked: new Date("2026-09-08T12:00:00Z"),
} satisfies Snapshot;

afterEach(() => vi.unstubAllGlobals());

it("renders bounded top-domain and top-client statistics", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: { window_start: "2026-09-07T12:00:00Z", window_end: "2026-09-08T12:00:00Z", total: 3, blocked: 1, top_domains: [{ value: "example.test.", count: 2 }], top_clients: [{ value: "192.0.2.1", count: 3 }] } }) }));
  render(<Dashboard data={snapshot} history={[]} queryLoggingEnabled />);
  expect(await screen.findByText("example.test.")).toBeInTheDocument();
  expect(screen.getByText("192.0.2.1")).toBeInTheDocument();
});

it("shows unavailable rankings without requesting history when logging is disabled", () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<Dashboard data={snapshot} history={[]} queryLoggingEnabled={false} />);
  expect(screen.getAllByText(/Unavailable while query logging is disabled/)).toHaveLength(2);
  expect(fetch).not.toHaveBeenCalled();
});
