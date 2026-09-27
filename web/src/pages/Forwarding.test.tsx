import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Forwarding } from "./Forwarding";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const rules = [
  {
    id: 1, domain: "corp.example", upstreams: ["10.0.0.10:53", "10.0.0.11:53"], enabled: true, description: "Office AD",
    health: [
      { address: "10.0.0.10:53", state: "healthy", consecutive_failures: 0, latency_milliseconds: 4.2 },
      { address: "10.0.0.11:53", state: "unavailable", consecutive_failures: 3, latency_milliseconds: 0 },
    ],
  },
];

function stubFetch(list: unknown = rules) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    let data: unknown = list;
    if (path.endsWith("/test")) data = { name: "corp.example.", type: "A", rcode: "NOERROR", answers: ["192.0.2.10"], upstream: "10.0.0.10:53", duration_ms: 3.5 };
    else if (init?.method && init.method !== "GET") data = rules[0];
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("lists rules with per-resolver health", async () => {
  stubFetch();
  render(withI18n(<Forwarding />));
  expect(await screen.findByText("corp.example")).toBeInTheDocument();
  expect(screen.getByText("10.0.0.11:53")).toBeInTheDocument();
  expect(screen.getByText("Healthy")).toBeInTheDocument();
  expect(screen.getByText("Unavailable")).toBeInTheDocument();
  expect(screen.getByText("Office AD")).toBeInTheDocument();
});

it("explains the empty state", async () => {
  stubFetch([]);
  render(withI18n(<Forwarding />));
  expect(await screen.findByText("No forwarding rules")).toBeInTheDocument();
});

it("creates a rule from comma-separated resolvers", async () => {
  const fetch = stubFetch();
  render(withI18n(<Forwarding />));
  await screen.findByText("corp.example");
  fireEvent.click(screen.getByRole("button", { name: "Add rule" }));
  fireEvent.change(screen.getByLabelText("Domain"), { target: { value: " lab.example " } });
  fireEvent.change(screen.getByLabelText(/Resolvers/), { target: { value: "10.0.1.1, 10.0.1.2:5353" } });
  fireEvent.click(screen.getByRole("button", { name: "Save rule" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/forwarding" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ domain: "lab.example", upstreams: ["10.0.1.1", "10.0.1.2:5353"], description: "", enabled: true });
  });
  expect(await screen.findByText(/Forwarding rule saved/)).toBeInTheDocument();
});

it("runs a diagnostic query through a rule", async () => {
  const fetch = stubFetch();
  render(withI18n(<Forwarding />));
  fireEvent.click(await screen.findByLabelText("Test corp.example"));
  fireEvent.click(screen.getByRole("button", { name: "Run test" }));
  expect(await screen.findByText("192.0.2.10")).toBeInTheDocument();
  const call = fetch.mock.calls.find(([path]) => path === "/api/v1/forwarding/1/test");
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "corp.example", type: "A" });
});

it("toggles a rule and keeps viewers read-only", async () => {
  const fetch = stubFetch();
  const { unmount } = render(withI18n(<Forwarding />));
  fireEvent.click(await screen.findByRole("switch", { name: "Enable or disable rule for corp.example" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/forwarding/1" && init?.method === "PUT");
    expect(JSON.parse(String(call?.[1]?.body)).enabled).toBe(false);
  });
  unmount();
  render(withI18n(<Forwarding readOnly />));
  expect(await screen.findByRole("switch", { name: "Enable or disable rule for corp.example" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Add rule" })).toBeDisabled();
  expect(screen.getByLabelText("Test corp.example")).toBeDisabled();
});
