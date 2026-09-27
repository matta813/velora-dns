import { afterEach, expect, it } from "vitest";
import { watchScrollRegions } from "./scroll-regions";

afterEach(() => { document.body.innerHTML = ""; });

function sized(element: HTMLElement, scrollWidth: number, clientWidth: number) {
  Object.defineProperty(element, "scrollWidth", { configurable: true, value: scrollWidth });
  Object.defineProperty(element, "clientWidth", { configurable: true, value: clientWidth });
}

it("makes only overflowing tables focusable, labelled regions", () => {
  document.body.innerHTML = `<main>
    <div class="table-wrap" id="wide"><table><caption>Zones</caption></table></div>
    <div class="table-wrap" id="narrow"><table></table></div>
  </main>`;
  const wide = document.getElementById("wide")!;
  const narrow = document.getElementById("narrow")!;
  sized(wide, 900, 390);
  sized(narrow, 300, 390);
  const stop = watchScrollRegions(document.querySelector("main")!, "scrolls sideways");
  expect(wide.tabIndex).toBe(0);
  expect(wide.getAttribute("role")).toBe("region");
  expect(wide.getAttribute("aria-label")).toBe("Zones (scrolls sideways)");
  expect(narrow.hasAttribute("tabindex")).toBe(false);

  sized(wide, 390, 390);
  window.dispatchEvent(new Event("resize"));
  return new Promise<void>((done) => requestAnimationFrame(() => {
    expect(wide.hasAttribute("tabindex")).toBe(false);
    expect(wide.hasAttribute("role")).toBe(false);
    stop();
    done();
  }));
});
