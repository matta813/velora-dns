import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { Policies } from "./Policies";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const clients = {
  clients: [
    { id: 1, name: "Kid tablet", addresses: ["192.168.1.40"], group: "", description: "", enabled: true, activity: null },
    { id: 2, name: "Work laptop", addresses: ["192.168.1.50"], group: "", description: "", enabled: true, activity: null },
  ],
  unnamed: [],
};
const sources = [
  { id: 3, name: "Ads", url: "https://lists.example/ads.txt", enabled: true, last_error: "" },
  { id: 4, name: "Adult content", url: "", enabled: false, last_error: "" },
];
const policies = [{ id: 7, client_id: 2, mode: "disabled", blocklists: [], allow: [], block: [], enabled: true }];

function stubFetch(policyList: unknown = policies) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    let data: unknown = policies[0];
    if (!init?.method || init.method === "GET") {
      if (path === "/api/v1/clients") data = clients;
      else if (path === "/api/v1/blocklists") data = sources;
      else if (path === "/api/v1/policies") data = policyList;
      else if (path.startsWith("/api/v1/policies/effective")) data = { client: clients.clients[1], policy: policies[0], mode: "disabled" };
    }
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const renderPage = (props: { readOnly?: boolean } = {}) =>
  render(<MemoryRouter>{withI18n(<Policies {...props} />)}</MemoryRouter>);

it("lists policies by client name and mode", async () => {
  stubFetch();
  renderPage();
  expect(await screen.findByText("Work laptop")).toBeInTheDocument();
  expect(screen.getAllByText("No filtering").length).toBeGreaterThan(0);
  expect(screen.getByText("All queries are answered unfiltered")).toBeInTheDocument();
});

it("creates a custom policy with a globally disabled list", async () => {
  const fetch = stubFetch();
  renderPage();
  await screen.findByText("Work laptop");
  fireEvent.click(screen.getByRole("button", { name: "Add policy" }));
  // Clients that already have a policy are not offered again.
  expect(screen.getByLabelText("Client")).toHaveValue("1");
  expect(screen.queryByRole("option", { name: "Work laptop" })).not.toBeInTheDocument();
  expect(screen.getByText("(off globally)")).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText(/Adult content/));
  fireEvent.change(screen.getByLabelText(/Also block/), { target: { value: "games.example\nvideo.example" } });
  fireEvent.change(screen.getByLabelText(/Always allow/), { target: { value: "school.example" } });
  fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/policies" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ client_id: 1, mode: "custom", blocklists: [4], allow: ["school.example"], block: ["games.example", "video.example"], enabled: true });
  });
});

it("shows which policy applies to an address", async () => {
  stubFetch();
  renderPage();
  await screen.findByText("Work laptop");
  fireEvent.change(screen.getByLabelText("Check address"), { target: { value: "192.168.1.50" } });
  fireEvent.click(screen.getByRole("button", { name: "Check address" }));
  const status = await screen.findByText(/→ Work laptop/);
  expect(status).toBeInTheDocument();
});

it("explains that policies need named clients", async () => {
  vi.stubGlobal("fetch", vi.fn((path: string) => Promise.resolve({ ok: true, json: async () => ({ data: path === "/api/v1/clients" ? { clients: [], unnamed: [] } : [] }) })));
  renderPage({ readOnly: true });
  expect(await screen.findByText(/Name a device first/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Add policy" })).toBeDisabled();
});
