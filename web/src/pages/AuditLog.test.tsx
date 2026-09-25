import { render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AuditLog } from "./AuditLog";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

it("renders read-only audit outcomes and actor details", async () => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ data: [{ id: 1, occurred_at: "2026-01-01T12:00:00Z", actor: "admin", role: "admin", action: "PUT", target: "/api/v1/config", result: "success", status_code: 200 }] }),
  })));
  render(withI18n(<AuditLog />));
  expect(await screen.findByText("admin (admin)")).toBeInTheDocument();
  expect(screen.getByText("/api/v1/config")).toBeInTheDocument();
  expect(screen.getByText("Success 200")).toBeInTheDocument();
});
