import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Login } from "./Login";

afterEach(() => vi.unstubAllGlobals());

it("submits credentials and hands off the authenticated identity", async () => {
  const session = { user: { id: 1, username: "admin", role: "admin", active: true, created_at: "2026-09-08T00:00:00Z" }, expires_at: "2026-09-09T00:00:00Z" };
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: session }) });
  vi.stubGlobal("fetch", fetch);
  const onLogin = vi.fn();
  render(<Login onLogin={onLogin} />);
  fireEvent.change(screen.getByLabelText("Username"), { target: { value: "admin" } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "correct horse battery staple" } });
  fireEvent.click(screen.getByText("Sign in"));
  await waitFor(() => expect(onLogin).toHaveBeenCalledWith(session));
  const options = fetch.mock.calls[0][1] as RequestInit;
  expect(options.method).toBe("POST");
  expect(options.body).toBe(JSON.stringify({ username: "admin", password: "correct horse battery staple" }));
});
