import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { CommandPalette, type PaletteItem } from "./CommandPalette";
import { withI18n } from "../test-i18n";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

function renderPalette(actions: PaletteItem[] = []) {
  const onClose = vi.fn();
  const pages: PaletteItem[] = [
    { id: "zones", label: "Local zones", hint: "DNS", icon: null, keywords: "records", run: vi.fn() },
    { id: "clients", label: "Clients", hint: "Network", icon: null, run: vi.fn() },
  ];
  render(
    <MemoryRouter>
      {withI18n(
        <>
          <button>before</button>
          <CommandPalette open onClose={onClose} pages={pages} actions={actions} />
          <Routes><Route path="*" element={<Location />} /></Routes>
        </>,
      )}
    </MemoryRouter>,
  );
  return { onClose, pages };
}

it("filters pages and actions and runs the highlighted option with Enter", () => {
  const theme = vi.fn();
  const { onClose, pages } = renderPalette([{ id: "theme", label: "Switch to dark theme", icon: null, run: theme }]);
  const input = screen.getByRole("combobox", { name: /Search pages/ });
  expect(input).toHaveFocus();
  expect(screen.getAllByRole("option")).toHaveLength(3);
  fireEvent.change(input, { target: { value: "record" } });
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option")).toHaveAttribute("aria-selected", "true");
  fireEvent.keyDown(input, { key: "Enter" });
  expect(pages[0].run).toHaveBeenCalled();
  expect(onClose).toHaveBeenCalled();

  fireEvent.change(input, { target: { value: "" } });
  fireEvent.keyDown(input, { key: "End" });
  expect(input).toHaveAttribute("aria-activedescendant", screen.getAllByRole("option")[2].id);
  fireEvent.keyDown(input, { key: "ArrowDown" });
  expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
  fireEvent.keyDown(input, { key: "ArrowUp" });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(theme).toHaveBeenCalled();
});

it("debounces resource search and opens results", async () => {
  vi.useFakeTimers();
  const fetch = vi.fn((path: string) => Promise.resolve({ ok: true, json: async () => ({ data: path.includes("q=hom")
    ? [{ kind: "zone", title: "home.arpa", subtitle: "ns.home.arpa", link: "/zones?zone=7" }, { kind: "client", title: "Home Assistant", subtitle: "192.0.2.21", link: "/clients?name=Home+Assistant" }]
    : [] }) }));
  vi.stubGlobal("fetch", fetch);
  renderPalette();
  const input = screen.getByRole("combobox");
  fireEvent.change(input, { target: { value: "h" } });
  fireEvent.change(input, { target: { value: "ho" } });
  fireEvent.change(input, { target: { value: "hom" } });
  await act(async () => { await vi.advanceTimersByTimeAsync(250); });
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe("/api/v1/search?q=hom");
  vi.useRealTimers();
  expect(await screen.findByText("home.arpa")).toBeInTheDocument();
  expect(screen.getByText("Zones")).toBeInTheDocument();
  expect(screen.getByText("Clients", { selector: ".palette-group" })).toBeInTheDocument();
  fireEvent.click(screen.getByText("Home Assistant"));
  expect(screen.getByTestId("location")).toHaveTextContent("/clients?name=Home+Assistant");
});

it("closes with Escape, keeps Tab inside and restores focus", async () => {
  const before = () => screen.getByRole("button", { name: "before" });
  const onClose = vi.fn();
  const view = render(<MemoryRouter>{withI18n(<><button>before</button></>)}</MemoryRouter>);
  before().focus();
  view.rerender(<MemoryRouter>{withI18n(<><button>before</button><CommandPalette open onClose={onClose} pages={[]} actions={[]} /></>)}</MemoryRouter>);
  const input = screen.getByRole("combobox");
  expect(input).toHaveFocus();
  expect(screen.getByRole("dialog")).toHaveAttribute("aria-modal", "true");
  fireEvent.keyDown(input, { key: "Tab" });
  expect(input).toHaveFocus();
  fireEvent.keyDown(input, { key: "Escape" });
  expect(onClose).toHaveBeenCalled();
  view.rerender(<MemoryRouter>{withI18n(<><button>before</button><CommandPalette open={false} onClose={onClose} pages={[]} actions={[]} /></>)}</MemoryRouter>);
  await waitFor(() => expect(before()).toHaveFocus());
});
