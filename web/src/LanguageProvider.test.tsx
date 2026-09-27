import { render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LanguageProvider } from "./LanguageProvider";

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("sets the document language for assistive technology", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
  localStorage.setItem("velora_language", "de");
  render(<LanguageProvider><p>content</p></LanguageProvider>);
  await waitFor(() => expect(document.documentElement.lang).toBe("de"));
});
