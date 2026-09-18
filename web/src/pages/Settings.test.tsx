import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Settings } from "./Settings";
import type { Snapshot } from "../api";

it("describes the active management authentication model", () => {
  const data = {
    status: {
      ready: true,
      uptime_seconds: 1,
      dns_listen: ["127.0.0.1:5353"],
      version: { version: "dev", commit: "unknown", built: "unknown" },
      capabilities: [],
    },
    stats: {
      queries_total: 0,
      blocked_queries: 0,
      queries_per_second: 0,
      cache_hit_rate: 0,
    },
    cache: { entries: 0, capacity: 100, hits: 0, misses: 0 },
    config: {
      query_log: { enabled: false, retention: 0, max_rows: 1, queue_size: 1 },
      dns: {
        listen: ["127.0.0.1:5353"],
        upstreams: ["1.1.1.1:53"],
        allowed_clients: ["127.0.0.0/8"],
        timeout: 2_000_000_000,
        retries: 1,
        max_concurrent: 256,
      },
      cache: { max_entries: 100 },
      http: { listen: "127.0.0.1:8080", web_dir: "web/dist" },
      log_level: "info",
    },
    checked: new Date(),
  } satisfies Snapshot;

  render(<Settings data={data} />);

  expect(
    screen.getByText(/requires an authenticated user or scoped API token/i),
  ).toBeInTheDocument();
  expect(screen.queryByText(/authentication is planned/i)).not.toBeInTheDocument();
});
