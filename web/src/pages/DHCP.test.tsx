import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DHCP } from "./DHCP";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const pool = { id: 1, name: "lan", interface: "eth0", subnet: "192.168.1.0/24", gateway: "192.168.1.1", dns_servers: ["1.1.1.1"], lease_seconds: 86400, enabled: true, created_at: "", updated_at: "" };
const lease = (id: number, over: Record<string, unknown> = {}) => ({
  id, pool_id: 1, mac_address: `aa:bb:cc:dd:ee:0${id}`, ip_address: `192.168.1.${100 + id}`, hostname: "", client_id: "",
  expires_at: new Date(Date.now() + 2.5 * 3600000).toISOString(), status: "active", created_at: "", ...over,
});

function stubFetch(data: Record<string, unknown> = {}, fail?: { status: number; message: string }) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    if (fail) return Promise.resolve({ ok: false, status: fail.status, json: async () => ({ error: { message: fail.message } }) });
    const body = (init?.method ?? "GET") === "GET" ? data[path] : {};
    return Promise.resolve({ ok: true, json: async () => ({ data: body }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const full = {
  "/api/v1/dhcp/pools": [pool],
  "/api/v1/dhcp/leases": [lease(1, { hostname: "laptop" }), lease(2), lease(3, { status: "expired" })],
};
const renderPage = (readOnly = false) => render(withI18n(<DHCP readOnly={readOnly} />));

it("shows a loading state first", () => {
  vi.stubGlobal("fetch", vi.fn(() => new Promise(() => {})));
  renderPage();
  expect(screen.getByText("Loading DHCP data…")).toBeInTheDocument();
});

it("shows empty states when the API returns null", async () => {
  stubFetch({ "/api/v1/dhcp/pools": null, "/api/v1/dhcp/leases": null });
  renderPage();
  expect(await screen.findByText(/No DHCP pools configured/)).toBeInTheDocument();
  expect(screen.getByText("No active leases.")).toBeInTheDocument();
});

it("lists active leases with expiry and per-pool count", async () => {
  stubFetch(full);
  renderPage();
  expect(await screen.findByText("laptop")).toBeInTheDocument();
  expect(screen.getByText("aa:bb:cc:dd:ee:01")).toBeInTheDocument();
  expect(screen.getByText("192.168.1.101")).toBeInTheDocument();
  expect(screen.getAllByText(/^2h \d+m$/)).toHaveLength(2);
  expect(screen.queryByText("aa:bb:cc:dd:ee:03")).not.toBeInTheDocument();
  expect(within(screen.getByText("lan").closest("tr")!).getByText("2")).toBeInTheDocument();
});

it("hides write controls when read-only", async () => {
  stubFetch(full);
  renderPage(true);
  await screen.findByText("laptop");
  expect(screen.queryByRole("button", { name: "Add Pool" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /^Delete/ })).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("lan"));
  expect(await screen.findByText("Static Reservations")).toBeInTheDocument();
  expect(screen.queryByLabelText("MAC address")).not.toBeInTheDocument();
});

it("shows write controls when editable", async () => {
  stubFetch(full);
  renderPage();
  await screen.findByText("laptop");
  expect(screen.getByRole("button", { name: "Add Pool" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Delete lan" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Delete 192.168.1.101" })).toBeInTheDocument();
});

it("shows API errors in an alert", async () => {
  stubFetch({}, { status: 500, message: "database offline" });
  renderPage();
  expect(await screen.findByRole("alert")).toHaveTextContent("database offline");
});

it("creates a pool with the expected body", async () => {
  const fetch = stubFetch(full);
  renderPage();
  await screen.findByText("laptop");
  fireEvent.click(screen.getByRole("button", { name: "Add Pool" }));
  fireEvent.change(screen.getByLabelText("Pool name"), { target: { value: "iot" } });
  fireEvent.change(screen.getByLabelText("Subnet (CIDR)"), { target: { value: "10.0.0.0/24" } });
  fireEvent.change(screen.getByLabelText("Gateway"), { target: { value: "10.0.0.1" } });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/dhcp/pools" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "iot", interface: "eth0", subnet: "10.0.0.0/24", gateway: "10.0.0.1", dns_servers: ["1.1.1.1"], lease_seconds: 86400, enabled: true });
  });
});
