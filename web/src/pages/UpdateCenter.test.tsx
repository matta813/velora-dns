import { render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withI18n } from "../test-i18n";
import { UpdateCenter } from "./UpdateCenter";

afterEach(() => vi.unstubAllGlobals());

it("shows discovered release details and enables installation", async () => {
  const responses: Record<string, unknown> = {
    "/api/v1/update/status": { state: "idle", installed: "1.0.0", updating: false },
    "/api/v1/update/history": [],
    "/api/v1/update/check": {
      installed_version: "1.0.0",
      latest_version: "1.1.0",
      update_available: true,
      channel: "stable",
      release_date: "2026-09-20T12:00:00Z",
      release_notes: "Security and reliability fixes",
      architecture: "linux/amd64",
      download_size: 10485760,
    },
  };
  vi.stubGlobal("fetch", vi.fn((path: string) => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: responses[path] }),
  })));

  render(withI18n(<UpdateCenter readOnly={false} />));

  expect(await screen.findByText("Update available: 1.1.0")).toBeInTheDocument();
  expect(screen.getByText(/stable · linux\/amd64/)).toBeInTheDocument();
  expect(screen.getByText("Security and reliability fixes")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Request Update" })).toBeEnabled();
});

it("shows real update phases and detailed failure state", async () => {
  const responses: Record<string, unknown> = {
    "/api/v1/update/status": {
      state: "installing", installed: "1.0.0", from_version: "1.0.0",
      to_version: "1.1.0", updating: true, error: "readiness failed",
    },
    "/api/v1/update/history": [],
    "/api/v1/update/check": {
      installed_version: "1.0.0", latest_version: "1.1.0", update_available: true,
      channel: "stable", architecture: "linux/amd64",
    },
  };
  vi.stubGlobal("fetch", vi.fn((path: string) => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: responses[path] }),
  })));

  render(withI18n(<UpdateCenter readOnly={false} />));

  expect(await screen.findByRole("list", { name: "Update progress" })).toHaveTextContent("✓ Downloading");
  expect(screen.getByRole("list", { name: "Update progress" })).toHaveTextContent("● Installing");
  expect(screen.getByRole("alert")).toHaveTextContent("readiness failed");
});
