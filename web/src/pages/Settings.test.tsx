import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Settings } from "./Settings";
import { withI18n } from "../test-i18n";
import { SUPPORTED_THEMES } from "../theme";
import type { Snapshot } from "../api";

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
    http: { listen: "127.0.0.1:8080", web_dir: "web/dist", allowed_hosts: ["localhost", "127.0.0.1", "::1"] },
    log_level: "info",
  },
  checked: new Date(),
} satisfies Snapshot;

it("describes the active management authentication model", () => {
  render(withI18n(<Settings data={data} />));

  expect(
    screen.getByText(/configuration is saved to the yaml config file/i),
  ).toBeInTheDocument();
});

it("offers the supported theme choices", () => {
  render(withI18n(<Settings data={data} />));
  const select = screen.getByLabelText("Theme");
  for (const theme of SUPPORTED_THEMES) {
    expect(select).toHaveTextContent(theme === "auto" ? "Auto" : theme === "light" ? "Light" : "Dark");
  }
});
