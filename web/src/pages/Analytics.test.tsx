import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { Analytics } from "./Analytics";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const bucket = (hour: number, total: number, blocked = 0) => ({
  start: `2026-09-27T${String(hour).padStart(2, "0")}:00:00Z`, total, blocked, cached: Math.floor(total / 2), failed: 0, average_ms: total ? 20 : 0,
});

function analytics(range: string) {
  const series = Array.from({ length: 24 }, (_, i) => bucket(i, i === 23 ? 40 : i % 3, i === 23 ? 4 : 0));
  return {
    range, window_start: "2026-09-26T13:00:00Z", window_end: "2026-09-27T12:30:00Z", bucket_seconds: 3600, history_start: "2026-09-27T06:00:00Z",
    totals: { start: "2026-09-26T13:00:00Z", total: 100, blocked: 8, cached: 50, failed: 2, average_ms: 21.5 },
    series,
    query_types: [{ value: "A", count: 70 }, { value: "AAAA", count: 30 }],
    response_codes: [{ value: "NOERROR", count: 90 }],
    sources: [{ value: "cache", count: 50 }, { value: "upstream", count: 42 }],
    upstreams: [{ address: "1.1.1.1:53", queries: 42, failed: 2, average_ms: 21.5 }],
    top_blocked: [{ value: "ads.example.", count: 8 }],
  };
}

function stubFetch() {
  const fetch = vi.fn((path: string) => {
    const url = new URL(path, "http://localhost");
    const data = url.pathname === "/api/v1/analytics"
      ? analytics(url.searchParams.get("range") ?? "24h")
      : { window_start: "", window_end: "", total: 100, blocked: 8, top_domains: [{ value: "docs.example.", count: 12 }], top_clients: [{ value: "192.0.2.4", count: 60, name: "Laptop" }] };
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const renderPage = (queryLogging = true) =>
  render(<MemoryRouter>{withI18n(<Analytics queryLogging={queryLogging} />)}</MemoryRouter>);

it("shows totals, breakdowns and upstream usage", async () => {
  stubFetch();
  renderPage();
  expect(await screen.findByText("21.5 ms", { selector: "strong" })).toBeInTheDocument();
  expect(screen.getByText("50.0%", { selector: "strong" })).toBeInTheDocument();
  expect(screen.getByText("ads.example.")).toBeInTheDocument();
  expect(screen.getByText("1.1.1.1:53")).toBeInTheDocument();
  expect(screen.getByText("Laptop")).toBeInTheDocument();
  expect(screen.getByText(/Query history only goes back to/)).toBeInTheDocument();
  expect(screen.getByRole("img", { name: /Queries per time bucket/ })).toBeInTheDocument();
});

it("switches ranges through the API", async () => {
  const fetch = stubFetch();
  renderPage();
  await screen.findByText("ads.example.");
  fireEvent.click(screen.getByRole("button", { name: "7 days" }));
  await waitFor(() => {
    expect(fetch.mock.calls.some(([path]) => path === "/api/v1/analytics?range=7d")).toBe(true);
    expect(fetch.mock.calls.some(([path]) => path === "/api/v1/query-stats?window=7d&limit=10")).toBe(true);
  });
  expect(screen.getByRole("button", { name: "7 days" })).toHaveAttribute("aria-pressed", "true");
});

it("lets keyboard users inspect buckets", async () => {
  stubFetch();
  renderPage();
  const chart = await screen.findByRole("img", { name: /Queries per time bucket/ });
  const detail = chart.closest("section")?.querySelector(".chart-detail");
  expect(detail).toHaveTextContent("Queries: 40");
  fireEvent.keyDown(chart, { key: "ArrowLeft" });
  expect(detail).toHaveTextContent("Queries: 1");
});

it("explains that analytics need query logging", () => {
  const fetch = stubFetch();
  renderPage(false);
  expect(screen.getByText("Query logging is off")).toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});
