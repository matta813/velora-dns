import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Zones } from "../pages/Zones";
import type { Zone } from "./types";
afterEach(() => vi.unstubAllGlobals());
const zone: Zone = {
  id: 1,
  name: "home.arpa.",
  primary_ns: "ns.home.arpa.",
  contact: "hostmaster.home.arpa.",
  revision: 1,
  records: [
    {
      id: 7,
      name: "router.home.arpa.",
      type: "A",
      ttl: 300,
      value: "192.168.1.1",
      priority: 0,
    },
  ],
};
function mock(data: unknown, status = 200) {
  return {
    ok: status < 400,
    status,
    json: async () =>
      status < 400 ? { data } : { error: { message: "Zone changed" } },
  };
}
it("shows load failures without pretending the zone list is empty", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("Offline")));
  render(<Zones />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Offline");
  expect(
    screen.queryByText("Give your network familiar names"),
  ).not.toBeInTheDocument();
});
it("creates a zone and confirms deletion with its current revision", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(mock([]))
    .mockResolvedValueOnce(mock({ ...zone, records: [] }, 201))
    .mockResolvedValueOnce(mock({ deleted: 1 }));
  vi.stubGlobal("fetch", fetch);
  render(<Zones />);
  fireEvent.click(await screen.findByText("Create your first zone"));
  fireEvent.change(screen.getByLabelText("Zone name"), {
    target: { value: "home.arpa" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create zone" }));
  await screen.findByText("Zone created. Add records to start using it.");
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({
    name: "home.arpa",
  });
  fireEvent.click(screen.getByRole("button", { name: "Delete zone" }));
  expect(fetch).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByText("Delete zone permanently"));
  await screen.findByText("Zone and its records deleted.");
  expect(fetch.mock.calls[2][1].headers["If-Match"]).toBe('"1"');
});
it("preserves an edited draft on revision conflict and reloads after cancel", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(mock([zone]))
    .mockResolvedValueOnce(mock(null, 412))
    .mockResolvedValueOnce(mock([{ ...zone, revision: 2 }]));
  vi.stubGlobal("fetch", fetch);
  render(<Zones />);
  fireEvent.click(await screen.findByLabelText("Edit router A record"));
  fireEvent.change(screen.getByLabelText("IPv4 address"), {
    target: { value: "192.168.1.2" },
  });
  expect(screen.getByText("Reload zones")).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Save record" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Your draft is unchanged",
  );
  expect(screen.getByLabelText("IPv4 address")).toHaveValue("192.168.1.2");
  expect(fetch.mock.calls[1][0]).toBe("/api/v1/zones/1/records/7");
  expect(fetch.mock.calls[1][1].headers["If-Match"]).toBe('"1"');
  expect(screen.getByText("192.168.1.1")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  fireEvent.click(screen.getByText("Reload zones"));
  await screen.findByText(/Revision 2/);
});
it("adds and removes records using the revision returned by the server", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(mock([{ ...zone, records: [] }]))
    .mockResolvedValueOnce(mock({ ...zone, revision: 2 }, 201))
    .mockResolvedValueOnce(mock({ ...zone, records: [], revision: 3 }));
  vi.stubGlobal("fetch", fetch);
  render(<Zones />);
  await screen.findByText(/Revision 1/);
  fireEvent.click(screen.getByRole("button", { name: "Add record" }));
  fireEvent.change(screen.getByLabelText(/Record name/), {
    target: { value: "router" },
  });
  fireEvent.change(screen.getByLabelText("IPv4 address"), {
    target: { value: "192.168.1.1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save record" }));
  fireEvent.click(await screen.findByLabelText("Delete router A record"));
  fireEvent.click(screen.getByText("Delete record permanently"));
  await waitFor(() =>
    expect(screen.queryByText("192.168.1.1")).not.toBeInTheDocument(),
  );
  expect(fetch.mock.calls[2][1].headers["If-Match"]).toBe('"2"');
});
