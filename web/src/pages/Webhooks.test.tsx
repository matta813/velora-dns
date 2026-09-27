import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Webhooks } from "./Webhooks";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const hooks = [
  {
    id: 1, name: "Home Assistant", url: "http://192.168.1.5:8123/api/webhook/velora", events: ["upstream.unavailable"], min_severity: "warning",
    allow_private: true, enabled: true, has_token: true, last_delivery_at: "2026-09-27T10:00:00Z", last_status: "HTTP 200", last_error: "", consecutive_failures: 0,
  },
  {
    id: 2, name: "Pager", url: "https://pager.example/hook", events: [], min_severity: "info",
    allow_private: false, enabled: true, has_token: false, last_delivery_at: "2026-09-27T11:00:00Z", last_status: "failed", last_error: "endpoint returned HTTP 500", consecutive_failures: 3,
  },
];
const types = ["upstream.unavailable", "upstream.recovered", "backup.failed"];

function stubFetch(testOK = true) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    let data: unknown = hooks[0];
    if (path === "/api/v1/webhooks/event-types") data = types;
    else if (path === "/api/v1/webhooks" && (!init?.method || init.method === "GET")) data = hooks;
    else if (path.endsWith("/test")) data = testOK ? { ok: true, error: "", webhook: hooks[0] } : { ok: false, error: "timed out", webhook: hooks[1] };
    return Promise.resolve({ ok: true, json: async () => ({ data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

const renderPage = () => render(withI18n(<Webhooks />));

it("shows delivery status and failures", async () => {
  stubFetch();
  renderPage();
  expect(await screen.findByText("Home Assistant")).toBeInTheDocument();
  expect(screen.getByText("HTTP 200")).toBeInTheDocument();
  expect(screen.getByText("Failing ×3")).toBeInTheDocument();
  expect(screen.getByText("endpoint returned HTTP 500")).toBeInTheDocument();
  expect(screen.getByText("All events")).toBeInTheDocument();
});

it("creates a webhook with events and a token", async () => {
  const fetch = stubFetch();
  renderPage();
  await screen.findByText("Home Assistant");
  fireEvent.click(screen.getByRole("button", { name: "Add webhook" }));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Chat" } });
  fireEvent.change(screen.getByLabelText("URL"), { target: { value: "https://chat.example/hook" } });
  fireEvent.change(screen.getByLabelText(/Bearer token/), { target: { value: "secret" } });
  fireEvent.click(screen.getByLabelText("backup.failed"));
  fireEvent.click(screen.getByRole("button", { name: "Save webhook" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/webhooks" && init?.method === "POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ name: "Chat", url: "https://chat.example/hook", events: ["backup.failed"], min_severity: "info", allow_private: false, enabled: true, token: "secret" });
  });
});

it("keeps a stored token when editing unless it is changed", async () => {
  const fetch = stubFetch();
  window.scrollTo = vi.fn();
  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "Edit webhook Home Assistant" }));
  expect(screen.getByText("Token stored")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Save webhook" }));
  await waitFor(() => {
    const call = fetch.mock.calls.find(([path, init]) => path === "/api/v1/webhooks/1" && init?.method === "PUT");
    expect(JSON.parse(String(call?.[1]?.body))).not.toHaveProperty("token");
  });
});

it("reports test results", async () => {
  stubFetch(false);
  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "Send test Pager" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Test failed for Pager: timed out");
});
