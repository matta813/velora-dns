#!/usr/bin/env node
// Accessibility and responsive audit for the management UI.
//
// Serves nothing itself: point it at a running UI (for example
// `npx vite preview --port 4173`) and it loads every page with demo API
// responses at phone, tablet and desktop widths, then reports:
//   - axe-core violations (WCAG 2.2 A/AA rules)
//   - horizontal page overflow
//   - touch targets smaller than 24×24 CSS px (WCAG 2.5.8)
//   - keyboard focus that is not visibly indicated
// Exit status is 1 when any page has a problem.
//
//   node scripts/a11y-audit.mjs --base-url http://127.0.0.1:4173 [--pages /zones]
/* global document, getComputedStyle -- used inside page.evaluate */
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { parseArgs } from "node:util";
import { demoResponse } from "./fixtures.mjs";

const require = createRequire(import.meta.url);
const axeSource = readFileSync(require.resolve("axe-core/axe.min.js"), "utf8");

const ROUTES = [
  "/", "/queries", "/analytics", "/zones", "/blocklists", "/policies", "/rewrites", "/forwarding", "/cache",
  "/clients", "/dhcp", "/cluster", "/settings", "/updates", "/backup", "/diagnostics", "/events", "/webhooks", "/audit",
];
const VIEWPORTS = [
  { name: "phone", width: 390, height: 844 },
  { name: "tablet", width: 820, height: 1180 },
  { name: "desktop", width: 1440, height: 900 },
];

const { values } = parseArgs({
  options: {
    "base-url": { type: "string", default: "http://127.0.0.1:4173" },
    pages: { type: "string", multiple: true },
    executable: { type: "string" },
    theme: { type: "string", multiple: true },
  },
});

const { chromium } = await import("playwright");
const browser = await chromium.launch({ executablePath: values.executable || process.env.CHROMIUM_PATH || undefined });
const routes = values.pages?.length ? values.pages : ROUTES;
const themes = values.theme?.length ? values.theme : ["light", "dark"];
let problems = 0;

for (const theme of themes) for (const viewport of VIEWPORTS) {
  const context = await browser.newContext({ viewport, locale: "en-US", timezoneId: "UTC", reducedMotion: "reduce" });
  await context.route("**/api/v1/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    const data = pathname === "/api/v1/preferences" ? { language: "en", theme } : demoResponse(pathname);
    await route.fulfill(data === null
      ? { status: 404, contentType: "application/json", body: JSON.stringify({ error: { message: "no fixture" } }) }
      : { status: 200, contentType: "application/json", body: JSON.stringify({ data }) });
  });
  await context.addInitScript((selected) => {
    localStorage.setItem("velora_language", "en");
    localStorage.setItem("velora_theme", selected);
  }, theme);
  for (const path of routes) {
    const page = await context.newPage();
    await page.goto(values["base-url"] + path, { waitUntil: "networkidle" });
    await page.locator("main").waitFor();
    await page.waitForTimeout(300);
    const findings = [];

    await page.addScriptTag({ content: axeSource });
    const axe = await page.evaluate(async () => {
      const result = await globalThis.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } });
      return result.violations.map((v) => `${v.id} (${v.impact}): ${v.nodes.slice(0, 3).map((n) => n.target.join(" ")).join(" | ")}`);
    });
    findings.push(...axe.map((v) => `axe ${v}`));

    const layout = await page.evaluate(() => {
      const out = [];
      const width = document.documentElement.clientWidth;
      if (document.documentElement.scrollWidth > width + 1) {
        const wide = [...document.querySelectorAll("body *")]
          .filter((el) => el.getBoundingClientRect().right > width + 1 && !el.closest(".table-wrap, pre, .chart-table"))
          .slice(0, 3)
          .map((el) => el.tagName.toLowerCase() + (el.className && typeof el.className === "string" ? "." + el.className.split(" ").join(".") : ""));
        out.push(`horizontal overflow ${document.documentElement.scrollWidth}px > ${width}px: ${wide.join(", ")}`);
      }
      const interactive = document.querySelectorAll("a[href], button, input:not([type=hidden]), select, textarea, summary, [role=switch], [tabindex]:not([tabindex='-1'])");
      for (const el of interactive) {
        const rect = el.getBoundingClientRect();
        const style = getComputedStyle(el);
        if (!rect.width || !rect.height || style.visibility === "hidden") continue;
        // Inline links inside text are exempt from the target-size rule.
        if (el.tagName === "A" && style.display === "inline" && el.closest("p, li, td, small, span")) continue;
        // A checkbox or radio inside a large enough label can be tapped via the label.
        const labelTarget = [...(el.labels ?? [])].some((label) => {
          const box = label.getBoundingClientRect();
          return box.width >= 24 && box.height >= 24;
        });
        if ((rect.width < 24 || rect.height < 24) && !labelTarget) {
          const label = el.getAttribute("aria-label") || el.textContent.trim().slice(0, 30) || el.tagName.toLowerCase();
          out.push(`small target ${Math.round(rect.width)}×${Math.round(rect.height)}: ${label}`);
        }
      }
      return out;
    });
    findings.push(...layout);

    // Tab through the first controls and make sure focus is visible.
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press("Tab");
      const focus = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const style = getComputedStyle(el);
        const visible = (style.outlineStyle !== "none" && parseFloat(style.outlineWidth) > 0) || style.boxShadow !== "none";
        return visible ? null : (el.getAttribute("aria-label") || el.textContent.trim().slice(0, 30) || el.tagName.toLowerCase());
      });
      if (focus) findings.push(`focus not visible: ${focus}`);
    }

    // The command palette is a modal dialog; audit it open on the overview.
    if (path === "/") {
      await page.keyboard.press("Control+K");
      await page.getByRole("combobox").waitFor();
      await page.keyboard.type("home");
      await page.waitForTimeout(400);
      const palette = await page.evaluate(async () => {
        const result = await globalThis.axe.run(document.querySelector(".palette"), { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } });
        return result.violations.map((v) => `${v.id} (${v.impact}): ${v.nodes.slice(0, 3).map((n) => n.target.join(" ")).join(" | ")}`);
      });
      findings.push(...palette.map((v) => `palette axe ${v}`));
      await page.keyboard.press("Escape");
    }

    const unique = [...new Set(findings)];
    if (unique.length) {
      problems += unique.length;
      console.log(`\n${theme} ${viewport.name} ${path}`);
      for (const finding of unique) console.log(`  - ${finding}`);
    }
    await page.close();
  }
  await context.close();
}
await browser.close();
console.log(problems ? `\n${problems} problem(s) found.` : "No accessibility or layout problems found.");
process.exit(problems ? 1 : 0);
