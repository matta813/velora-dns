import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
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

afterEach(() => vi.unstubAllGlobals());

it("describes the active management authentication model", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            data: { enabled: false, global_qps: 1000, client_qps: 100, rate_limit_burst: 100 },
          }),
      }),
    ),
  );
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

it("loads and updates rate limit settings", async () => {
  interface FetchCall {
    method?: string;
    body?: string;
  }
  const calls: FetchCall[] = [];
  const fetch = vi.fn((_path: string, options: FetchCall) => {
    calls.push(options);
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({
          data:
            options?.method === "PUT"
              ? { enabled: true, global_qps: 500, client_qps: 50, rate_limit_burst: 25 }
              : { enabled: false, global_qps: 1000, client_qps: 100, rate_limit_burst: 100 },
        }),
    });
  });
  vi.stubGlobal("fetch", fetch);
  render(withI18n(<Settings data={data} />));
  const toggle = await screen.findByRole("checkbox", {
    name: "Enable rate limiting",
  });
  fireEvent.click(toggle);
  fireEvent.change(screen.getByLabelText("Global queries per second"), {
    target: { value: "500" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
  const put = calls.find((c) => c.method === "PUT");
  expect(JSON.parse(put!.body ?? "{}")).toEqual({
    enabled: true,
    global_qps: 500,
    client_qps: 100,
    rate_limit_burst: 100,
  });
});

it("allows typing commas in upstream DNS servers and saves parsed configuration", async () => {
  interface FetchCall {
    path?: string;
    method?: string;
    body?: string;
  }
  const calls: FetchCall[] = [];
  const fetch = vi.fn((path: string, options?: { method?: string; body?: string }) => {
    calls.push({ path, method: options?.method, body: options?.body });
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({
          data: { status: "saved", message: "Configuration saved." },
        }),
    });
  });
  vi.stubGlobal("fetch", fetch);
  render(withI18n(<Settings data={data} />));

  const input = screen.getByLabelText(/upstream dns servers/i);
  fireEvent.change(input, {
    target: { value: "1.1.1.1:53, 8.8.8.8:53, " },
  });
  expect(input).toHaveValue("1.1.1.1:53, 8.8.8.8:53, ");

  fireEvent.click(screen.getByRole("button", { name: "Save configuration" }));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/v1/config" && c.method === "PUT")).toBe(true),
  );
  const put = calls.find((c) => c.path === "/api/v1/config" && c.method === "PUT");
  const payload = JSON.parse(put!.body ?? "{}");
  expect(payload.dns.upstreams).toEqual(["1.1.1.1:53", "8.8.8.8:53"]);
});

it("preserves custom DNS listeners and the web address when saving", async () => {
  const calls: { path: string; method?: string; body?: string }[] = [];
  vi.stubGlobal("fetch", vi.fn((path: string, options?: { method?: string; body?: string }) => {
    calls.push({ path, method: options?.method, body: options?.body });
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ data: { status: "saved" } }) });
  }));
  const custom = {
    ...data,
    config: {
      ...data.config,
      dns: { ...data.config.dns, listen: ["127.0.0.1:5353", "[::1]:5353"] },
      http: { ...data.config.http, listen: "192.168.1.2:9090" },
    },
  };
  render(withI18n(<Settings data={custom} />));
  expect(screen.getByLabelText(/dns listen addresses/i)).toHaveValue("127.0.0.1:5353, [::1]:5353");
  expect(screen.getByLabelText(/web ui listen address/i)).toHaveValue("192.168.1.2:9090");
  fireEvent.click(screen.getByRole("button", { name: "Save configuration" }));
  await waitFor(() => expect(calls.some((call) => call.path === "/api/v1/config" && call.method === "PUT")).toBe(true));
  const payload = JSON.parse(calls.find((call) => call.path === "/api/v1/config" && call.method === "PUT")!.body!);
  expect(payload.dns.listen).toEqual(["127.0.0.1:5353", "[::1]:5353"]);
  expect(payload.http.listen).toBe("192.168.1.2:9090");
});
