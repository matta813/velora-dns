import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CachePage } from "./CachePage";
import { withI18n } from "../test-i18n";
import type { Snapshot } from "../api";

afterEach(() => vi.unstubAllGlobals());

it("shows cached names, answers, and remaining TTL", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ data: { total: 1, entries: [{ name: "example.test.", type: "A", rcode: "NOERROR", answers: ["192.0.2.1"], remaining_ttl: 3600 }] } }),
  }));
  const data = { cache: { entries: 1, capacity: 100, hits: 1, misses: 0 }, checked: new Date() } as Snapshot;
  render(withI18n(<CachePage data={data} refresh={() => {}} />));
  expect(await screen.findByText("example.test.")).toBeInTheDocument();
  expect(screen.getByText("192.0.2.1")).toBeInTheDocument();
  expect(screen.getByText("3600 s")).toBeInTheDocument();
});

const snapshot = { cache: { entries: 1, capacity: 100, hits: 1, misses: 0 }, checked: new Date() } as Snapshot;
const onePage = { total: 1, entries: [{ name: "example.test.", type: "A", rcode: "NOERROR", answers: ["192.0.2.1"], remaining_ttl: 60 }] };

function stubFetch() {
  const fetch = vi.fn((path: string, ...rest: [RequestInit?]) => {
    void rest;
    const body = path.startsWith("/api/v1/cache/invalidate") ? { removed: 1, stats: {} } : onePage;
    return Promise.resolve({ ok: true, json: async () => ({ data: body }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("filters cache entries by name and record type", async () => {
  const fetch = stubFetch();
  render(withI18n(<CachePage data={snapshot} refresh={() => {}} />));
  await screen.findByText("example.test.");
  fireEvent.change(screen.getByLabelText("Search by name"), { target: { value: "example" } });
  fireEvent.change(screen.getByLabelText("Type"), { target: { value: "AAAA" } });
  await waitFor(() => {
    const last = new URL(String(fetch.mock.calls.at(-1)?.[0]), "http://localhost");
    expect(last.searchParams.get("domain")).toBe("example");
    expect(last.searchParams.get("type")).toBe("AAAA");
  });
});

it("removes a single cached entry", async () => {
  const fetch = stubFetch();
  const refresh = vi.fn();
  render(withI18n(<CachePage data={snapshot} refresh={refresh} />));
  fireEvent.click(await screen.findByLabelText("Remove from cache: example.test. A"));
  await screen.findByText("Removed cached entries: 1");
  const call = fetch.mock.calls.find(([path]) => path === "/api/v1/cache/invalidate");
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "example.test.", type: "A", include_subdomains: false });
  expect(refresh).toHaveBeenCalled();
});

it("asks before flushing the whole cache", async () => {
  const fetch = stubFetch();
  const confirm = vi.fn().mockReturnValue(false);
  vi.stubGlobal("confirm", confirm);
  render(withI18n(<CachePage data={snapshot} refresh={() => {}} />));
  await screen.findByText("example.test.");
  fireEvent.click(screen.getByRole("button", { name: "Clear cache" }));
  expect(confirm).toHaveBeenCalled();
  expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
});

it("disables cache mutations for viewers", async () => {
  stubFetch();
  render(withI18n(<CachePage data={snapshot} refresh={() => {}} readOnly />));
  expect(await screen.findByLabelText("Remove from cache: example.test. A")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Clear cache" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Invalidate" })).toBeDisabled();
});
