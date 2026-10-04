import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { EventCenter } from "./EventCenter";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

it("shows event severity and marks an event as read", async () => {
  const onRead = vi.fn();
  let read = false;
  const fetch = vi.fn().mockImplementation((url: string) => {
    if (url.endsWith("/read")) {
      read = true;
      return Promise.resolve({ ok: true, json: async () => ({ data: { read: true } }) });
    }
    return Promise.resolve({ ok: true, json: async () => ({ data: { unread_count: read ? 0 : 1, events: [{ id: 4, severity: "warning", title: "Upstream unavailable", message: "Failover active", link: "/", occurred_at: "2026-09-25T09:00:00Z", repeat_count: 2, read }] } }) });
  });
  vi.stubGlobal("fetch", fetch);
  render(withI18n(<MemoryRouter><EventCenter onRead={onRead} /></MemoryRouter>));
  expect(await screen.findByText("Upstream unavailable")).toBeInTheDocument();
  expect(screen.getByText("Warning")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("href", "/");
  fireEvent.click(screen.getByRole("button", { name: "Mark as read" }));
  await waitFor(() => expect(onRead).toHaveBeenCalledOnce());
  expect(fetch).toHaveBeenCalledWith("/api/v1/events/4/read", expect.objectContaining({ method: "POST" }));
  expect(screen.queryByRole("button", { name: "Mark as read" })).not.toBeInTheDocument();
});
