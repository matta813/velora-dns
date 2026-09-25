import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BackupAssistant } from "./BackupAssistant";
import { withI18n } from "../test-i18n";

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

it("downloads an encrypted backup only for an admin", async () => {
  const fetch = vi.fn().mockImplementation((url: string) => {
    if (url === "/api/v1/backup/create") return Promise.resolve({ ok: true, blob: async () => new Blob(["encrypted"]) });
    return Promise.resolve({ ok: true, json: async () => ({ data: { supported: true, verification_state: "unknown", database_path: "data/velora.db" } }) });
  });
  vi.stubGlobal("fetch", fetch);
  vi.stubGlobal("URL", { createObjectURL: vi.fn(() => "blob:backup"), revokeObjectURL: vi.fn() });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  render(withI18n(<BackupAssistant readOnly={false} canCreate />));
  const button = await screen.findByRole("button", { name: "Create and download" });
  expect(button).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Passphrase"), { target: { value: "a sufficiently long passphrase" } });
  fireEvent.click(button);
  await waitFor(() => expect(fetch).toHaveBeenCalledWith("/api/v1/backup/create", expect.objectContaining({ method: "POST" })));
});

it("hides the backup export action from non-admin users", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: { supported: true, verification_state: "unknown", database_path: "data/velora.db" } }) }));
  render(withI18n(<BackupAssistant readOnly canCreate={false} />));
  expect(await screen.findByText("Encrypted backup")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Create and download" })).not.toBeInTheDocument();
});
