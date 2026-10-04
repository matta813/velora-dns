import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RestorePanel } from "./RestorePanel";
import { withI18n } from "../test-i18n";

afterEach(() => vi.unstubAllGlobals());

const inspection = {
  token: "0123456789abcdef0123456789abcdef",
  expires_at: "2026-09-27T13:00:00Z",
  metadata: { format_version: 1, velora_version: "0.1.0-beta.10", created_at: "2026-09-20T10:00:00Z", schema_version: 18, components: ["configuration", "database"] },
  summary: {
    zones: 2, records: 14, blocklists: 3, clients: 4, rewrites: 1, forward_rules: 1, users: 2,
    dns_listen: ["0.0.0.0:53"], upstreams: ["1.1.1.1:53"], http_listen: "0.0.0.0:8080", current_version: "0.1.0-beta.11", current_schema: 21,
    warnings: ["The backup uses database schema 18; it will be upgraded to schema 21 when Velora starts."],
  },
};

type Handler = (path: string, init?: RequestInit) => { status?: number; data?: unknown; error?: { message: string } };

function stub(handler: Handler) {
  const fetch = vi.fn((path: string, init?: RequestInit) => {
    const reply = handler(path, init);
    const status = reply.status ?? 200;
    return Promise.resolve({ ok: status < 400, status, json: async () => (reply.error ? { error: reply.error } : { data: reply.data }) });
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

async function chooseBackup() {
  const file = new File(["VELBK001"], "velora-backup.vdns");
  fireEvent.change(screen.getByLabelText("Backup file (.vdns)"), { target: { files: [file] } });
  fireEvent.change(screen.getByLabelText("Backup passphrase"), { target: { value: "a sufficiently long passphrase" } });
  fireEvent.click(screen.getByRole("button", { name: "Check backup" }));
}

it("inspects before restoring and requires confirmation", async () => {
  const fetch = stub((path) => (path === "/api/v1/backup/inspect" ? { data: inspection } : { data: null }));
  render(withI18n(<RestorePanel />));
  await chooseBackup();
  expect(await screen.findByText("Backup is valid")).toBeInTheDocument();
  expect(screen.getByText(/2 zones · 14 records · 3 blocklists/)).toBeInTheDocument();
  expect(screen.getByText(/upgraded to schema 21/)).toBeInTheDocument();
  const body = fetch.mock.calls.find(([path]) => path === "/api/v1/backup/inspect")?.[1]?.body as FormData;
  expect([...body.keys()]).toEqual(["passphrase", "bundle"]);
  expect(screen.getByRole("button", { name: "Restore and restart" })).toBeDisabled();
  expect(fetch.mock.calls.some(([path, init]) => path === "/api/v1/backup/restore" && init?.method === "POST")).toBe(false);
});

it("restores, waits for the restart and reports the result", async () => {
  let restarted = false;
  const fetch = stub((path, init) => {
    if (path === "/api/v1/backup/inspect") return { data: inspection };
    if (path === "/api/v1/backup/restore" && init?.method === "POST") return { status: 202, data: { state: "restarting", metadata: inspection.metadata } };
    if (path === "/api/v1/backup/restore") {
      if (!restarted) {
        restarted = true;
        return { status: 502, error: { message: "Bad gateway" } };
      }
      return { data: { state: "rolled_back", metadata: inspection.metadata, error: "Velora did not become ready with the restored backup; the previous data was restored.", updated_at: new Date(Date.now() + 1000).toISOString() } };
    }
    return { data: null };
  });
  render(withI18n(<RestorePanel />));
  await chooseBackup();
  fireEvent.click(await screen.findByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Restore and restart" }));
  expect(await screen.findByText(/restarting to apply the backup/)).toBeInTheDocument();
  expect(await screen.findByText("Restore rolled back.", {}, { timeout: 6000 })).toBeInTheDocument();
  expect(screen.getByText(/previous data was restored/)).toBeInTheDocument();
  const post = fetch.mock.calls.find(([path, init]) => path === "/api/v1/backup/restore" && init?.method === "POST");
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({ token: inspection.token, confirm: true });
}, 10000);

it("tells the admin to sign in again when the restored database replaces sessions", async () => {
  stub((path, init) => {
    if (path === "/api/v1/backup/inspect") return { data: inspection };
    if (path === "/api/v1/backup/restore" && init?.method === "POST") return { status: 202, data: { state: "restarting", metadata: inspection.metadata } };
    if (path === "/api/v1/backup/restore") return { status: 401, error: { message: "Authentication required" } };
    return { data: null };
  });
  render(withI18n(<RestorePanel />));
  await chooseBackup();
  fireEvent.click(await screen.findByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Restore and restart" }));
  expect(await screen.findByText(/Sign in again with an account from the backup/, {}, { timeout: 5000 })).toBeInTheDocument();
});

it("shows why a backup was rejected", async () => {
  stub((path) => (path === "/api/v1/backup/inspect" ? { status: 400, error: { message: "The file is not a valid Velora backup, or the passphrase is wrong" } } : { data: null }));
  render(withI18n(<RestorePanel />));
  await chooseBackup();
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("passphrase is wrong"));
  expect(screen.queryByText("Backup is valid")).not.toBeInTheDocument();
});
