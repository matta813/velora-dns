import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Cluster } from "./Cluster";
import { withI18n } from "../test-i18n";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const baseState = {
  cluster_id: "", node_id: "aaaaaaaaaaaaaaaaaaaaaaaa", node_name: "", advertised_url: "", primary_url: "", allow_insecure: false,
  created_at: null, last_sync_at: null, last_sync_error: "", applied_revision: "",
};
const standalone = { state: { ...baseState, role: "standalone" }, revision: "", members: [], zones_read_only: false, protocol: 1, version: "1.0.0", replicated_zones: 0 };
const primary = {
  ...standalone,
  state: { ...baseState, role: "primary", cluster_id: "c1", node_name: "dns1", advertised_url: "https://dns1.example.lan" },
  revision: "ab12cd34",
  replicated_zones: 2,
  members: [
    { node_id: "b".repeat(24), name: "dns2", address: "192.0.2.2", version: "1.0.0", joined_at: "2026-09-27T10:00:00Z", last_seen_at: "2026-09-27T12:00:00Z", applied_revision: "ab12cd34", last_error: "", status: "in_sync" },
    { node_id: "c".repeat(24), name: "dns3", address: "192.0.2.3", version: "0.9.0", joined_at: "2026-09-27T10:00:00Z", last_seen_at: "2026-09-27T11:00:00Z", applied_revision: "", last_error: "zone lab.test.: invalid", status: "stale" },
  ],
};
const replica = { ...standalone, state: { ...baseState, role: "replica", cluster_id: "c1", node_name: "dns2", primary_url: "https://dns1.example.lan", last_sync_error: "cannot reach the primary: timed out" }, zones_read_only: true };

function stub(routes: Record<string, unknown>) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${path.split("?")[0]}`;
    const data = key in routes ? routes[key] : [];
    return Promise.resolve({ ok: true, status: 200, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("lets an admin create a cluster from a standalone node", async () => {
  const fetch = stub({ "GET /api/v1/cluster/overview": standalone, "POST /api/v1/cluster/create": primary });
  render(withI18n(<Cluster canManage />));
  const form = await screen.findByRole("form", { name: "Create a cluster" });
  fireEvent.change(screen.getAllByLabelText("Node name")[0], { target: { value: "dns1" } });
  fireEvent.change(screen.getByLabelText(/Address other nodes use/), { target: { value: "https://dns1.example.lan" } });
  fireEvent.submit(form);
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/cluster/create" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({ name: "dns1", advertised_url: "https://dns1.example.lan", skip_check: false });
  });
  expect(await screen.findByText(/The cluster was created/)).toBeInTheDocument();
  expect(screen.getByText("dns2")).toBeInTheDocument();
});

it("joins a primary only after confirming that zones are replaced", async () => {
  const fetch = stub({ "GET /api/v1/cluster/overview": standalone, "POST /api/v1/cluster/connect": replica });
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
  render(withI18n(<Cluster canManage />));
  const form = await screen.findByRole("form", { name: "Join a cluster" });
  fireEvent.change(screen.getByLabelText("Primary URL"), { target: { value: "https://dns1.example.lan" } });
  fireEvent.change(screen.getByLabelText("Join token"), { target: { value: "vjt_abc" } });
  fireEvent.change(screen.getAllByLabelText("Node name")[1], { target: { value: "dns2" } });
  fireEvent.submit(form);
  expect(fetch.mock.calls.some(([path]) => path === "/api/v1/cluster/connect")).toBe(false);
  fireEvent.submit(form);
  expect(confirm).toHaveBeenCalledTimes(2);
  await waitFor(() => expect(screen.getByText("Replicating from the primary")).toBeInTheDocument());
  expect(screen.getByRole("alert")).toHaveTextContent("timed out");
});

it("shows replicas, issues a one-time token and removes a replica after confirmation", async () => {
  const fetch = stub({
    "GET /api/v1/cluster/overview": primary,
    "POST /api/v1/cluster/join-tokens": { token: "vjt_secret-token", expires_at: "2026-09-27T12:30:00Z" },
    [`DELETE /api/v1/cluster/members/${"c".repeat(24)}`]: { ...primary, members: [primary.members[0]] },
  });
  vi.spyOn(window, "confirm").mockReturnValue(true);
  render(withI18n(<Cluster canManage />));
  expect(await screen.findByText("In sync")).toBeInTheDocument();
  expect(screen.getByText("Not responding")).toBeInTheDocument();
  expect(screen.getByText("zone lab.test.: invalid")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Add node" }));
  expect(await screen.findByText("vjt_secret-token")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Done" }));
  expect(screen.queryByText("vjt_secret-token")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Remove node dns3" }));
  await waitFor(() => expect(fetch.mock.calls.some(([path, init]) => String(path).startsWith("/api/v1/cluster/members/") && init?.method === "DELETE")).toBe(true));
  await waitFor(() => expect(screen.queryByText("dns3")).not.toBeInTheDocument());
});

it("offers sync and leave on a replica and nothing to viewers", async () => {
  const fetch = stub({ "GET /api/v1/cluster/overview": replica, "POST /api/v1/cluster/sync": { ...replica, state: { ...replica.state, last_sync_error: "" } } });
  const { unmount } = render(withI18n(<Cluster canManage />));
  fireEvent.click(await screen.findByRole("button", { name: "Sync now" }));
  expect(await screen.findByText("Replication is working.")).toBeInTheDocument();
  expect(fetch.mock.calls.some(([path, init]) => path === "/api/v1/cluster/sync" && init?.method === "POST")).toBe(true);
  unmount();
  render(withI18n(<Cluster />));
  await screen.findByText("Replicating from the primary");
  expect(screen.queryByRole("button", { name: "Sync now" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Leave cluster" })).not.toBeInTheDocument();
});
