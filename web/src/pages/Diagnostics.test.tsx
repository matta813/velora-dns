import { render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Diagnostics } from "./Diagnostics";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

it("shows backend health and offers a sanitized report download", async () => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: {
      generated_at: "2026-01-01T12:00:00Z",
      version: { version: "v1.2.3", commit: "abc", built: "2026-01-01" },
      os: "linux", architecture: "amd64", uptime_seconds: 12,
      state: "degraded", upstream_count: 2, query_log_enabled: false,
      components: [{ name: "updater", state: "degraded", detail: "Updater agent is unavailable" }],
    } }),
  })));
  render(withI18n(<Diagnostics />));
  expect(await screen.findByText(/Velora DNS v1.2.3/)).toBeInTheDocument();
  expect(screen.getByText("Updater agent is unavailable")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Download report" })).toBeInTheDocument();
});
