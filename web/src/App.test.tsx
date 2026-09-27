import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import App from "./App";
import { withI18n } from "./test-i18n";
import { AuthUserContext } from "./auth-context";
afterEach(() => {
  vi.unstubAllGlobals();
  document.title = "";
});
it("shows connection errors without fabricated dashboard numbers", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(new Error("Connection failed")),
  );
  render(
    <MemoryRouter>
      {withI18n(<App />)}
    </MemoryRouter>,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Connection failed",
  );
  expect(screen.queryByText("Total queries")).not.toBeInTheDocument();
  expect(screen.queryByTestId("running-version")).not.toBeInTheDocument();
});
it("renders server counters from the operational API", async () => {
  const responses: Record<string, unknown> = {
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
      {withI18n(<App />)}
    </MemoryRouter>,
  );
  expect(await screen.findByText("42")).toBeInTheDocument();
  expect(screen.getByText("50.0%")).toBeInTheDocument();
  expect(screen.getByText("Resolver online")).toBeInTheDocument();
  expect(screen.getByTestId("running-version")).toHaveTextContent("Velora DNS dev");
  expect(document.querySelector(".signout")).toHaveTextContent(
    "Sign out",
  );
});
it("updates the footer when the running backend version changes", async () => {
  let version = "v0.1.0-beta.9";
  const responses: Record<string, unknown> = {
    "/api/v1/stats": { queries_total: 0, queries_per_second: 0, cache_hit_rate: 0, blocked_queries: 0 },
    "/api/v1/cache": { entries: 0, capacity: 100, hits: 0, misses: 0 },
    "/api/v1/config": { query_log: { enabled: false }, dns: { upstreams: [] } },
  };
  vi.stubGlobal("fetch", vi.fn((path: string) => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: path === "/api/v1/status"
      ? { ready: true, version: { version }, capabilities: [], dns_listen: [], uptime_seconds: 1 }
      : responses[path] }),
  })));
  render(<MemoryRouter>{withI18n(<App />)}</MemoryRouter>);
  expect(await screen.findByTestId("running-version")).toHaveTextContent("Velora DNS v0.1.0-beta.9");
  version = "v0.1.0-beta.10";
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(screen.getByTestId("running-version")).toHaveTextContent("Velora DNS v0.1.0-beta.10"));
});
it("updates the document title for the current route", async () => {
  const responses: Record<string, unknown> = {
    "/api/v1/status": {
      ready: true,
      uptime_seconds: 1,
      dns_listen: ["127.0.0.1:5353"],
      version: { version: "dev" },
      capabilities: [],
    },
    "/api/v1/stats": {
      queries_total: 0,
      queries_per_second: 0,
      cache_hit_rate: 0,
      blocked_queries: 0,
    },
    "/api/v1/cache": { entries: 0, capacity: 100, hits: 0, misses: 0 },
    "/api/v1/config": {
      dns: {
        listen: ["127.0.0.1:5353"],
        upstreams: ["1.1.1.1:53"],
        allowed_clients: ["127.0.0.0/8"],
        timeout: 2000000000,
        retries: 1,
        max_concurrent: 256,
      },
      cache: { max_entries: 100, upstream_ttl: 86400 },
      http: { listen: "127.0.0.1:8080", web_dir: "web/dist", allowed_hosts: ["localhost", "127.0.0.1", "::1"] },
      query_log: { enabled: false, retention: 0, max_rows: 1, queue_size: 1 },
      log_level: "info",
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
    <MemoryRouter initialEntries={["/settings"]}>
      {withI18n(<App />)}
    </MemoryRouter>,
  );
  expect(await screen.findByText("Server configuration")).toBeInTheDocument();
  expect(document.title).toBe("Velora DNS · Settings");
});

it.each([
  ["/dhcp", "DHCP Server", "/api/v1/dhcp/pools", "No DHCP pools configured"],
  ["/cluster", "Cluster Overview", "/api/v1/cluster/nodes", "No cluster nodes registered"],
])("renders the %s application route", async (route, title, endpoint, emptyState) => {
  const responses: Record<string, unknown> = {
    "/api/v1/status": {
      ready: true,
      uptime_seconds: 1,
      dns_listen: ["127.0.0.1:5353"],
      version: { version: "dev" },
      capabilities: ["dhcp", "cluster"],
    },
    "/api/v1/stats": {
      queries_total: 0,
      queries_per_second: 0,
      cache_hit_rate: 0,
      blocked_queries: 0,
    },
    "/api/v1/cache": { entries: 0, capacity: 100, hits: 0, misses: 0 },
    "/api/v1/config": { query_log: { enabled: false } },
    "/api/v1/dhcp/pools": null,
    "/api/v1/dhcp/leases": null,
    "/api/v1/cluster/nodes": null,
    "/api/v1/cluster/config-versions?limit=20": null,
  };
  const fetchMock = vi.fn((path: string) =>
    Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ data: responses[path] }),
    }),
  );
  vi.stubGlobal("fetch", fetchMock);

  render(
    <MemoryRouter initialEntries={[route]}>
      {withI18n(<App />)}
    </MemoryRouter>,
  );

  expect(await screen.findByRole("heading", { level: 1, name: title })).toBeInTheDocument();
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(endpoint, expect.anything()));
  expect(await screen.findByText(new RegExp(emptyState))).toBeInTheDocument();
});

function stubEmptyApi() {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
}

it("opens navigation dropdowns from the keyboard and returns focus on Escape", async () => {
  stubEmptyApi();
  render(<MemoryRouter>{withI18n(<App />)}</MemoryRouter>);
  const toggle = await screen.findByRole("button", { name: /^DNS/ });
  toggle.focus();
  fireEvent.keyDown(toggle, { key: "ArrowDown" });
  const first = await screen.findByRole("link", { name: "Local zones" });
  await waitFor(() => expect(first).toHaveFocus());
  fireEvent.keyDown(first, { key: "ArrowDown" });
  expect(screen.getByRole("link", { name: "Blocklists" })).toHaveFocus();
  fireEvent.keyDown(document.activeElement!, { key: "End" });
  expect(screen.getByRole("link", { name: "DNS cache" })).toHaveFocus();
  fireEvent.keyDown(document.activeElement!, { key: "ArrowDown" });
  expect(first).toHaveFocus();
  fireEvent.keyDown(first, { key: "Escape" });
  expect(toggle).toHaveFocus();
  expect(screen.queryByRole("link", { name: "Local zones" })).not.toBeInTheDocument();
});

it("traps focus in the mobile menu and closes it with Escape", async () => {
  stubEmptyApi();
  render(<MemoryRouter>{withI18n(<App />)}</MemoryRouter>);
  const menuButton = await screen.findByRole("button", { name: "Open navigation" });
  fireEvent.click(menuButton);
  const nav = screen.getByRole("navigation", { name: /main/i });
  const links = nav.querySelectorAll<HTMLElement>("a[href], button");
  await waitFor(() => expect(links[0]).toHaveFocus());
  links[links.length - 1].focus();
  fireEvent.keyDown(window, { key: "Tab" });
  expect(screen.getByRole("button", { name: "Close navigation" })).toHaveFocus();
  fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
  expect(links[links.length - 1]).toHaveFocus();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.getByRole("button", { name: "Open navigation" })).toHaveFocus();
});

it("moves focus to the page heading after navigation", async () => {
  stubEmptyApi();
  render(<MemoryRouter>{withI18n(<App />)}</MemoryRouter>);
  fireEvent.click(await screen.findByRole("link", { name: "Query log" }));
  await waitFor(() => expect(screen.getByRole("heading", { level: 1 })).toHaveFocus());
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Query log");
});

function renderAs(role: "admin" | "viewer") {
  stubEmptyApi();
  render(
    <MemoryRouter>
      <AuthUserContext.Provider value={{ username: "demo", role, csrf_token: "x" }}>{withI18n(<App />)}</AuthUserContext.Provider>
    </MemoryRouter>,
  );
}

it("opens the command palette with Ctrl+K and offers role-appropriate actions", async () => {
  renderAs("admin");
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  const input = await screen.findByRole("combobox");
  expect(input).toHaveFocus();
  expect(screen.getByRole("option", { name: /Add a client/ })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: /Create a backup/ })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: /Audit log/ })).toBeInTheDocument();
  fireEvent.change(input, { target: { value: "query log" } });
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(screen.queryByRole("combobox")).not.toBeInTheDocument());
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Query log");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(await screen.findByRole("combobox")).toBeInTheDocument();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  await waitFor(() => expect(screen.queryByRole("combobox")).not.toBeInTheDocument());
});

it("gives viewers no write shortcuts or admin pages in the palette", async () => {
  renderAs("viewer");
  fireEvent.click(await screen.findByRole("button", { name: "Search" }));
  await screen.findByRole("combobox");
  expect(screen.queryByRole("option", { name: /Add a client/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Create a backup/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Audit log/ })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: /Check for updates/ })).toBeInTheDocument();
});
