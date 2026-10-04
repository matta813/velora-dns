import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Blocklists } from "./Blocklists";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const sources = [
  { id: 1, name: "Community hosts", url: "https://example.org/hosts.txt", enabled: true, last_updated_at: "2026-09-21T10:00:00Z", last_error: "", domain_count: 10, update_interval: 86400, last_attempt_at: "2026-09-21T10:00:00Z", consecutive_failures: 0, next_update_at: "2099-09-22T10:00:00Z" },
  { id: 2, name: "Fresh list", url: "https://example.org/new.txt", enabled: true, last_error: "", domain_count: 0, update_interval: 0, consecutive_failures: 0, next_update_at: null },
  { id: 3, name: "Local overrides", url: "", enabled: true, last_updated_at: "2026-09-18T09:30:00Z", last_error: "", domain_count: 2, update_interval: 0, consecutive_failures: 0, next_update_at: null },
];

function stubFetch() {
  const fetch = vi.fn((path: string, ...rest: [RequestInit?]) => {
    void rest;
    const data = path === "/api/v1/blocklists" ? sources : sources[0];
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("shows schedules, next update and never-updated sources", async () => {
  stubFetch();
  render(withI18n(<Blocklists />));
  expect(await screen.findByText("Community hosts")).toBeInTheDocument();
  expect(screen.getByLabelText("Update schedule for Community hosts")).toHaveValue("86400");
  expect(screen.getByLabelText("Update schedule for Fresh list")).toHaveValue("0");
  expect(screen.queryByLabelText("Update schedule for Local overrides")).not.toBeInTheDocument();
  expect(screen.getByText("Never updated")).toBeInTheDocument();
  expect(screen.getByText(new Date("2099-09-22T10:00:00Z").toLocaleString())).toBeInTheDocument();
});

it("changes a source schedule", async () => {
  const fetch = stubFetch();
  render(withI18n(<Blocklists />));
  fireEvent.change(await screen.findByLabelText("Update schedule for Fresh list"), { target: { value: "21600" } });
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/blocklists/2" && init?.method === "PUT");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ update_interval: 21600 });
  });
});

it("sends the chosen schedule when adding a URL source", async () => {
  const fetch = stubFetch();
  render(withI18n(<Blocklists />));
  await screen.findByText("Community hosts");
  fireEvent.click(screen.getByRole("button", { name: "Add source" }));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Ads" } });
  fireEvent.change(screen.getByLabelText("Source URL"), { target: { value: "https://example.net/ads.txt" } });
  fireEvent.change(screen.getByLabelText("Update schedule"), { target: { value: "3600" } });
  fireEvent.submit(screen.getByRole("form", { name: "Add blocklist source" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/blocklists" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "Ads", url: "https://example.net/ads.txt", update_interval: 3600 });
  });
});

it("keeps schedules read-only for viewers", async () => {
  stubFetch();
  render(withI18n(<Blocklists readOnly />));
  expect(await screen.findByLabelText("Update schedule for Community hosts")).toBeDisabled();
});
