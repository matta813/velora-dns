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
it("shows load errors without displaying fabricated empty results", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(new Error("Storage unavailable")),
  );
  render(<QueryLog enabled={false} />);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Storage unavailable",
  );
  expect(screen.queryByText("No matching queries")).not.toBeInTheDocument();
  expect(screen.getByText(/Query logging is disabled/)).toBeInTheDocument();
});
