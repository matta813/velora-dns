import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DHCP } from "./DHCP";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const pool = { id: 1, name: "lan", interface: "eth0", subnet: "10.0.0.0/24", gateway: "10.0.0.1", dns_servers: ["1.1.1.1"], lease_seconds: 86400, enabled: true, created_at: "", updated_at: "" };
const reservation = { id: 7, pool_id: 1, mac_address: "aa:bb:cc:dd:ee:ff", ip_address: "10.0.0.50", hostname: "printer" };

function stubFetch(opts: { leases?: unknown[]; failReservations?: boolean } = {}) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    let data: unknown = null;
    if (method === "GET" && path === "/api/v1/dhcp/pools") data = [pool];
    else if (method === "GET" && path === "/api/v1/dhcp/leases") data = opts.leases ?? [];
    else if (method === "GET" && path.endsWith("/reservations")) {
      if (opts.failReservations) return Promise.resolve({ ok: false, status: 500, statusText: "boom", json: async () => ({ error: "reservations boom" }), text: async () => "reservations boom" });
      data = [reservation];
    }
    return Promise.resolve({ ok: true, status: 200, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const renderPage = () => render(withI18n(<DHCP />));
const calls = (f: ReturnType<typeof stubFetch>, method: string, suffix: string) =>
  f.mock.calls.filter(([p, i]) => (i?.method ?? "GET") === method && String(p).endsWith(suffix));

it("loads reservations when a pool is selected", async () => {
  const fetch = stubFetch();
  renderPage();
  fireEvent.click(await screen.findByText("lan"));
  expect(await screen.findByText("printer")).toBeInTheDocument();
  expect(calls(fetch, "GET", "/pools/1/reservations")).toHaveLength(1);
});

it("shows an error when reservations fail to load", async () => {
  stubFetch({ failReservations: true });
  renderPage();
  fireEvent.click(await screen.findByText("lan"));
  await waitFor(() => expect(document.body.textContent).toContain("Management API returned HTTP 500"));
});

it("does not fetch reservations of a deleted pool", async () => {
  const fetch = stubFetch();
  vi.stubGlobal("confirm", () => true);
  renderPage();
  fireEvent.click(await screen.findByText("lan"));
  await screen.findByText("printer");
  fetch.mockClear();
  fireEvent.click(screen.getByRole("button", { name: /Delete lan/i }));
  await waitFor(() => expect(calls(fetch, "DELETE", "/pools/1")).toHaveLength(1));
  await waitFor(() => expect(calls(fetch, "GET", "/api/v1/dhcp/leases")).toHaveLength(1));
  expect(calls(fetch, "GET", "/reservations")).toHaveLength(0);
});

it("drops empty DNS entries when creating a pool", async () => {
  const fetch = stubFetch();
  renderPage();
  await screen.findByText("lan");
  fireEvent.click(screen.getByRole("button", { name: "Add Pool" }));
  const dns = screen.getByLabelText("DNS servers");
  fireEvent.change(dns, { target: { value: "1.1.1.1, " } });
  expect(dns).toHaveValue("1.1.1.1, ");
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => {
    const c = calls(fetch, "POST", "/api/v1/dhcp/pools")[0];
    expect(JSON.parse(String(c?.[1]?.body)).dns_servers).toEqual(["1.1.1.1"]);
  });
});

it("formats exactly 24h in days", async () => {
  const expires = new Date(Date.now() + 24 * 3600000 + 30000).toISOString();
  stubFetch({ leases: [{ id: 1, pool_id: 1, mac_address: "aa:aa:aa:aa:aa:aa", ip_address: "10.0.0.9", hostname: "h", client_id: "", expires_at: expires, status: "active", created_at: "" }] });
  renderPage();
  expect(await screen.findByText("1d 0h")).toBeInTheDocument();
});
