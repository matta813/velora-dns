import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AnswerExplainer } from "./AnswerExplainer";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(response: { ok: boolean; status?: number; body: unknown }) {
  const fetchMock = vi.fn<(url: string, init: { body: string }) => Promise<unknown>>(() => Promise.resolve({ ok: response.ok, status: response.status ?? 200, json: () => Promise.resolve(response.body) }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

it("explains a cached answer with its remaining TTL and rule", async () => {
  const hit = { stage: "cache", result: "hit", remaining_ttl: 118, depth: 0 };
  const fetchMock = stubFetch({
    ok: true,
    body: { data: {
      name: "example.org.", type: "A", client: "192.168.1.40", source: "cache", winner: hit, rcode: "NOERROR",
      answers: ["example.org.\t300\tIN\tA\t192.0.2.1"],
      steps: [
        { stage: "filter", result: "allowed", depth: 0, filter: { scope: "client_policy", client: "Kid", mode: "custom", allowed_by: [{ name: "client policy domains", id: 4 }] } },
        { stage: "rewrite", result: "no_match", depth: 0 },
        { stage: "zone", result: "no_match", depth: 0 },
        hit,
      ],
    } },
  });
  render(withI18n(<AnswerExplainer />));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: " example.org " } });
  fireEvent.change(screen.getByLabelText("Record type"), { target: { value: "AAAA" } });
  fireEvent.change(screen.getByLabelText(/Client address/), { target: { value: "192.168.1.40" } });
  fireEvent.click(screen.getByRole("button", { name: "Explain" }));

  const result = await screen.findByRole("region", { name: "Explanation" });
  expect(within(result).getByText(/Deciding stage: Cache/)).toBeInTheDocument();
  expect(within(result).getByText("Remaining TTL: 118 s")).toBeInTheDocument();
  expect(within(result).getByText(/Allowed by: client policy domains \(#4\)/)).toBeInTheDocument();
  expect(within(result).getAllByText("No match")).toHaveLength(2);
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/diagnostics/explain", expect.objectContaining({
    method: "POST",
    body: JSON.stringify({ name: "example.org", type: "AAAA", client: "192.168.1.40" }),
  }));
});

it("states that an upstream was not contacted and omits an empty client", async () => {
  const rule = { id: 3, name: "corp.example", detail: "10.0.0.10:53" };
  const fetchMock = stubFetch({
    ok: true,
    body: { data: {
      name: "app.corp.example.", type: "A", source: "upstream", rcode: "NOERROR", answers: [],
      winner: { stage: "forwarding", result: "would_forward", depth: 0, rules: [rule] },
      steps: [{ stage: "forwarding", result: "would_forward", depth: 0, rules: [rule] }],
    } },
  });
  render(withI18n(<AnswerExplainer />));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "app.corp.example" } });
  fireEvent.click(screen.getByRole("button", { name: "Explain" }));
  expect(await screen.findByText(/corp.example \(#3\)/)).toBeInTheDocument();
  expect(screen.getByText("No upstream was contacted.")).toBeInTheDocument();
  expect(screen.getByText(/No answer records/)).toBeInTheDocument();
  const init = fetchMock.mock.calls[0][1];
  expect(JSON.parse(init.body)).toEqual({ name: "app.corp.example", type: "A" });
});

it("shows the server's validation message", async () => {
  stubFetch({ ok: false, status: 400, body: { error: { code: "invalid_client", message: "Client must be an IPv4 or IPv6 address" } } });
  render(withI18n(<AnswerExplainer />));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "a.example" } });
  fireEvent.change(screen.getByLabelText(/Client address/), { target: { value: "nope" } });
  fireEvent.click(screen.getByRole("button", { name: "Explain" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Client must be an IPv4 or IPv6 address");
  expect(screen.queryByRole("region", { name: "Explanation" })).not.toBeInTheDocument();
});
