import { render, screen } from "@testing-library/react";
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
