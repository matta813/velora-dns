import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryLog } from "./QueryLog";
import { MemoryRouter } from "react-router-dom";
import { withI18n } from "../test-i18n";
afterEach(() => vi.unstubAllGlobals());
it("renders real API fields and submits all four filters only on apply", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [
          {
            id: 1,
            occurred_at: "2026-09-07T10:00:00Z",
            domain: "example.test.",
            client_ip: "192.0.2.1",
            type: "A",
            rcode: "NOERROR",
            source: "upstream",
            upstream: "1.1.1.1:53",
            duration: 1234000,
          },
        ],
      }),
    });
  vi.stubGlobal("fetch", fetch);
  render(<MemoryRouter>{withI18n(<QueryLog enabled />)}</MemoryRouter>);
  await screen.findByText("example.test.");
  expect(screen.getByText("1.23 ms")).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Domain"), {
    target: { value: "other.test" },
  });
  fireEvent.change(screen.getByLabelText("Client IP"), {
    target: { value: "192.0.2.2" },
  });
  fireEvent.change(screen.getByLabelText("Query type"), {
    target: { value: "AAAA" },
  });
  fireEvent.change(screen.getByLabelText("Source"), {
    target: { value: "blocked" },
  });
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByText("Apply filters"));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  const params = new URL(fetch.mock.calls[1][0], "http://localhost")
    .searchParams;
  expect(Object.fromEntries(params)).toMatchObject({
    domain: "other.test",
    client: "192.0.2.2",
    type: "AAAA",
    source: "blocked",
  });
});
it("shows an unavailable state without fetching when logging is disabled", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<MemoryRouter>{withI18n(<QueryLog enabled={false} />)}</MemoryRouter>);
  expect(await screen.findByRole("status")).toHaveTextContent(
    "Query history unavailable",
  );
  expect(screen.queryByText("No matching queries")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

it("opens the blocked-query view", async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: [] }) });
  vi.stubGlobal("fetch", fetch);
  render(<MemoryRouter>{withI18n(<QueryLog enabled />)}</MemoryRouter>);
  await screen.findByText("No matching queries");
  fireEvent.click(screen.getByText("Show blocked queries"));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(new URL(fetch.mock.calls[1][0], "http://localhost").searchParams.get("source")).toBe("blocked");
});

it("shows client names with a link and honours a client filter in the URL", async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: [
    { id: 9, occurred_at: "2026-09-21T12:00:00Z", client_ip: "192.168.1.200", client_name: "Living Room TV", domain: "tv.example.", type: "A", rcode: "NOERROR", source: "cache", upstream: "", duration: 1000000, cache_hit: true },
  ] }) });
  vi.stubGlobal("fetch", fetch);
  render(<MemoryRouter initialEntries={["/queries?client=192.168.1.200"]}>{withI18n(<QueryLog enabled />)}</MemoryRouter>);
  expect(await screen.findByRole("link", { name: "Living Room TV" })).toHaveAttribute("href", "/clients?name=Living%20Room%20TV");
  expect(new URL(fetch.mock.calls[0][0], "http://localhost").searchParams.get("client")).toBe("192.168.1.200");
  expect(screen.getByLabelText("Client IP")).toHaveValue("192.168.1.200");
});
