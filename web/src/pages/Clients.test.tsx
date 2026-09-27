import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { Clients } from "./Clients";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const list = {
  clients: [
    { id: 1, name: "Living Room TV", addresses: ["192.168.1.200"], group: "Media", description: "", enabled: true, activity: { queries: 1520, last_seen: "2026-09-21T12:00:00Z" } },
    { id: 2, name: "Home network", addresses: ["192.168.1.0/24"], group: "", description: "Everything else", enabled: true, activity: { queries: 0, last_seen: null } },
  ],
  unnamed: [{ client_ip: "192.168.1.77", queries: 88, last_seen: "2026-09-21T11:00:00Z" }],
};

function stubFetch(data: unknown = list) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    void path;
    const body = !init?.method || init.method === "GET" ? data : list.clients[0];
    return Promise.resolve({ ok: true, json: async () => ({ data: body }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const renderPage = (props: { readOnly?: boolean } = {}, path = "/clients") =>
  render(<MemoryRouter initialEntries={[path]}>{withI18n(<Clients {...props} />)}</MemoryRouter>);

it("lists clients with activity and links to their queries", async () => {
  stubFetch();
  renderPage();
  expect(await screen.findByText("Living Room TV")).toBeInTheDocument();
  expect(screen.getByText("Media")).toBeInTheDocument();
  expect(screen.getByText("No queries")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "View queries" })).toHaveAttribute("href", "/queries?client=192.168.1.200");
});

it("highlights the client named in the URL", async () => {
  stubFetch();
  window.HTMLElement.prototype.scrollIntoView = vi.fn();
  renderPage({}, "/clients?name=Living%20Room%20TV");
  const name = await screen.findByText("Living Room TV");
  expect(name.closest("tr")).toHaveClass("selected");
});

it("names an unnamed address from recent activity", async () => {
  const fetch = stubFetch();
  window.scrollTo = vi.fn();
  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "Name this device" }));
  expect(screen.getByLabelText(/Addresses/)).toHaveValue("192.168.1.77");
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Kitchen tablet" } });
  fireEvent.click(screen.getByRole("button", { name: "Save client" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/clients" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "Kitchen tablet", addresses: ["192.168.1.77"], group: "", description: "", enabled: true });
  });
});

it("shows the empty state and stays read-only for viewers", async () => {
  stubFetch({ clients: [], unnamed: list.unnamed });
  renderPage({ readOnly: true });
  expect(await screen.findByText("No named clients")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Add client" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Name this device" })).toBeDisabled();
});
