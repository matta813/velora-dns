import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { Rewrites } from "./Rewrites";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const rewrites = [
  { id: 1, name: "nas.home", type: "A", value: "192.0.2.5", enabled: true, description: "Storage", overrides_zone: "home" },
  { id: 2, name: "*.lab.home", type: "AAAA", value: "2001:db8::10", enabled: true, description: "" },
  { id: 3, name: "ads.home", type: "A", value: "192.0.2.6", enabled: false, description: "", blocked_by: "blocklist" },
];

function stubFetch(list: unknown = rewrites) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    const data = !init?.method || init.method === "GET" ? list : rewrites[0];
    void path;
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("lists rewrites with precedence hints", async () => {
  stubFetch();
  render(<MemoryRouter>{withI18n(<Rewrites />)}</MemoryRouter>);
  expect(await screen.findByText("nas.home")).toBeInTheDocument();
  expect(screen.getByText("Overrides local zone: home")).toBeInTheDocument();
  expect(screen.getByText("Blocked first by a blocklist")).toBeInTheDocument();
  expect(screen.getByRole("switch", { name: "Enable or disable rewrite for ads.home A" })).toHaveAttribute("aria-checked", "false");
});

it("filters by name or answer", async () => {
  stubFetch();
  render(<MemoryRouter>{withI18n(<Rewrites />)}</MemoryRouter>);
  await screen.findByText("nas.home");
  fireEvent.change(screen.getByPlaceholderText("Filter rewrites"), { target: { value: "2001:db8" } });
  expect(screen.queryByText("nas.home")).not.toBeInTheDocument();
  expect(screen.getByText("*.lab.home")).toBeInTheDocument();
});

it("creates a CNAME rewrite", async () => {
  const fetch = stubFetch();
  render(<MemoryRouter>{withI18n(<Rewrites />)}</MemoryRouter>);
  await screen.findByText("nas.home");
  fireEvent.click(screen.getByRole("button", { name: "Add rewrite" }));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: " docs.home " } });
  fireEvent.change(screen.getByLabelText("Type"), { target: { value: "CNAME" } });
  expect(screen.getByText("Target host name, resolved normally")).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText(/^Answer/), { target: { value: "nas.home" } });
  fireEvent.click(screen.getByRole("button", { name: "Save rewrite" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/rewrites" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "docs.home", type: "CNAME", value: "nas.home", enabled: true, description: "" });
  });
  expect(await screen.findByText("Rewrite saved and applied.")).toBeInTheDocument();
});

it("confirms before deleting and stays read-only for viewers", async () => {
  const fetch = stubFetch();
  vi.stubGlobal("confirm", vi.fn().mockReturnValue(false));
  const { unmount } = render(<MemoryRouter>{withI18n(<Rewrites />)}</MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Delete nas.home A"));
  expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  unmount();
  render(<MemoryRouter>{withI18n(<Rewrites readOnly />)}</MemoryRouter>);
  expect(await screen.findByLabelText("Delete nas.home A")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Add rewrite" })).toBeDisabled();
});

it("starts filtered when opened from search", async () => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: true, json: async () => ({ data: [
    { id: 1, name: "nas.home.arpa", type: "A", value: "192.0.2.20", enabled: true, description: "" },
    { id: 2, name: "tv.home.arpa", type: "A", value: "192.0.2.30", enabled: true, description: "" },
  ] }) })));
  render(<MemoryRouter initialEntries={["/rewrites?q=nas"]}>{withI18n(<Rewrites />)}</MemoryRouter>);
  expect(await screen.findByText("nas.home.arpa")).toBeInTheDocument();
  expect(screen.queryByText("tv.home.arpa")).not.toBeInTheDocument();
  expect(screen.getByRole("searchbox")).toHaveValue("nas");
});
