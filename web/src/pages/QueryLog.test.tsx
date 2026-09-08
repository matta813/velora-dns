import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryLog } from "./QueryLog";
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
  render(<QueryLog enabled />);
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
  render(<QueryLog enabled={false} />);
  expect(await screen.findByRole("status")).toHaveTextContent(
    "Query history unavailable",
  );
  expect(screen.queryByText("No matching queries")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

it("opens the blocked-query view", async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: [] }) });
  vi.stubGlobal("fetch", fetch);
  render(<QueryLog enabled />);
  await screen.findByText("No matching queries");
  fireEvent.click(screen.getByText("Show blocked queries"));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(new URL(fetch.mock.calls[1][0], "http://localhost").searchParams.get("source")).toBe("blocked");
});
