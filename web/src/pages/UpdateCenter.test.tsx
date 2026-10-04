import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withI18n } from "../test-i18n";
import { UpdateCenter } from "./UpdateCenter";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const available = {
  installed_version: "1.0.0", latest_version: "1.1.0", update_available: true, channel: "stable",
  release_date: "2026-09-20T12:00:00Z", release_notes: "Security and reliability fixes", architecture: "linux/amd64", download_size: 10485760,
};

type Reply = unknown | (() => unknown);

function stubFetch(responses: Record<string, Reply>) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${path}`;
    const reply = responses[key];
    const data = typeof reply === "function" ? (reply as () => unknown)() : reply;
    if (data instanceof Error) return Promise.reject(data);
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

it("shows only the check action until a check runs", async () => {
  const fetch = stubFetch({ "GET /api/v1/update/status": { state: "idle", installed: "1.0.0", updating: false }, "GET /api/v1/update/check": available });
  render(withI18n(<UpdateCenter readOnly={false} />));
  expect(await screen.findByText("Check whether a newer version is available.")).toBeInTheDocument();
  expect(screen.getAllByRole("button")).toHaveLength(1);
  expect(fetch.mock.calls.some(([path]) => path === "/api/v1/update/check")).toBe(false);

  fireEvent.click(screen.getByRole("button", { name: "Check for updates" }));
  expect(await screen.findByText("Update available: 1.1.0")).toBeInTheDocument();
  expect(screen.getByText("Security and reliability fixes")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Update" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "Check for updates" })).not.toBeInTheDocument();
});

it("reports up to date without offering an update", async () => {
  stubFetch({
    "GET /api/v1/update/status": { state: "completed", installed: "1.1.0", updating: false },
    "GET /api/v1/update/check": { ...available, installed_version: "1.1.0", update_available: false },
  });
  render(withI18n(<UpdateCenter readOnly={false} />));
  fireEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
  expect(await screen.findByText("The system is up to date.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
});

it("runs an update from the UI, shows progress and reports completion", async () => {
  const states = [
    { state: "completed", installed: "1.0.0", updating: false, last_completed: "2026-01-01T00:00:00Z" },
    { state: "completed", installed: "1.0.0", updating: false, last_completed: "2026-01-01T00:00:00Z" },
    { state: "installing", installed: "1.0.0", from_version: "1.0.0", to_version: "1.1.0", updating: true },
    new Error("connection refused"),
    { state: "completed", installed: "1.1.0", updating: false, last_completed: new Date(Date.now() + 60000).toISOString() },
  ];
  let calls = 0;
  const fetch = stubFetch({
    "GET /api/v1/update/status": () => states[Math.min(calls++, states.length - 1)],
    "GET /api/v1/update/check": available,
    "POST /api/v1/update/request": { status: "accepted" },
  });
  const onUpdated = vi.fn();
  render(withI18n(<UpdateCenter readOnly={false} onUpdated={onUpdated} />));
  fireEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
  fireEvent.click(await screen.findByRole("button", { name: "Update" }));
  // The last run's "completed" state is ignored until the new update starts.
  expect(await screen.findByText("Starting the update…")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("list", { name: "Update progress" })).toHaveTextContent("● Installing"), { timeout: 4000 });
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  expect(await screen.findByText(/Velora is restarting/, {}, { timeout: 4000 })).toBeInTheDocument();
  expect(await screen.findByText("Updated to 1.1.0", {}, { timeout: 4000 })).toBeInTheDocument();
  expect(onUpdated).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Reload page" })).toBeInTheDocument();
  expect(fetch.mock.calls.filter(([path, init]) => path === "/api/v1/update/request" && init?.method === "POST")).toHaveLength(1);
}, 15000);

it("resumes progress when opened during an update", async () => {
  stubFetch({
    "GET /api/v1/update/status": { state: "rolled_back", installed: "1.0.0", updating: true, error: "readiness failed" },
  });
  render(withI18n(<UpdateCenter readOnly={false} />));
  // Opened mid-update: progress resumes without a new request.
  expect(await screen.findByRole("list", { name: "Update progress" })).toBeInTheDocument();
});

it("keeps viewers read-only", async () => {
  stubFetch({ "GET /api/v1/update/status": { state: "idle", installed: "1.0.0", updating: false }, "GET /api/v1/update/check": available });
  render(withI18n(<UpdateCenter readOnly />));
  fireEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
  expect(await screen.findByText(/signed in as a viewer/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
});

it("explains request errors", async () => {
  stubFetch({
    "GET /api/v1/update/status": { state: "idle", installed: "1.0.0", updating: false },
    "GET /api/v1/update/check": available,
    "POST /api/v1/update/request": new Error("An update is already running"),
  });
  render(withI18n(<UpdateCenter readOnly={false} />));
  fireEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
  fireEvent.click(await screen.findByRole("button", { name: "Update" }));
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("An update is already running"));
  expect(screen.getByRole("button", { name: "Update" })).toBeEnabled();
});
