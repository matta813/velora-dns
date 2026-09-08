import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import App from "./App";
afterEach(() => vi.unstubAllGlobals());
it("shows connection errors without fabricated dashboard numbers", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(new Error("Connection failed")),
  );
  render(
    <MemoryRouter>
      <App />
    </MemoryRouter>,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Connection failed",
  );
  expect(screen.queryByText("Total queries")).not.toBeInTheDocument();
});
it("renders server counters from the operational API", async () => {
  const responses: Record<string, unknown> = {
    "/api/v1/auth/session": {
      user: { id: 1, username: "admin", role: "admin", active: true, created_at: "2026-09-08T00:00:00Z" },
      expires_at: "2026-09-09T00:00:00Z",
    },
    "/api/v1/status": {
      ready: true,
      uptime_seconds: 3600,
      dns_listen: ["127.0.0.1:5353"],
      version: { version: "dev" },
      capabilities: ["cache"],
    },
    "/api/v1/stats": {
      queries_total: 42,
      queries_per_second: 0.7,
      cache_hit_rate: 0.5,
      blocked_queries: 0,
    },
    "/api/v1/cache": { entries: 10, capacity: 100, hits: 21, misses: 21 },
    "/api/v1/config": {
      dns: { upstreams: ["1.1.1.1:53"], timeout: 2000000000 },
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn((path: string) =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ data: responses[path] }),
      }),
    ),
  );
  render(
    <MemoryRouter>
      <App />
    </MemoryRouter>,
  );
  expect(await screen.findByText("42")).toBeInTheDocument();
  expect(screen.getByText("50.0%")).toBeInTheDocument();
  expect(screen.getByText("Resolver online")).toBeInTheDocument();
});
