/**
 * Tables scroll horizontally inside `.table-wrap` on small screens. Keyboard
 * users can only scroll a region that can take focus, so every wrapper that
 * actually overflows becomes a labelled, focusable region; the others stay
 * out of the tab order.
 */
export function watchScrollRegions(root: HTMLElement, label: string): () => void {
  const update = () => {
    for (const element of root.querySelectorAll<HTMLElement>(".table-wrap")) {
      const scrollable = element.scrollWidth > element.clientWidth + 1;
      if (scrollable && element.tabIndex !== 0) {
        element.tabIndex = 0;
        element.setAttribute("role", "region");
        const caption = element.querySelector("caption")?.textContent?.trim();
        element.setAttribute("aria-label", caption ? `${caption} (${label})` : label);
      } else if (!scrollable && element.getAttribute("role") === "region") {
        element.removeAttribute("tabindex");
        element.removeAttribute("role");
        element.removeAttribute("aria-label");
      }
    }
  };
  let frame = 0;
  const schedule = () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(update);
  };
  const mutations = new MutationObserver(schedule);
  mutations.observe(root, { childList: true, subtree: true });
  window.addEventListener("resize", schedule);
  update();
  return () => {
    cancelAnimationFrame(frame);
    mutations.disconnect();
    window.removeEventListener("resize", schedule);
  };
}
