#!/usr/bin/env node

/**
 * Capture screenshots of the Velora DNS management interface.
 *
 * Usage:
 *   node scripts/screenshot.mjs                    # capture all pages
 *   node scripts/screenshot.mjs --pages dashboard zones  # specific pages
 *   node scripts/screenshot.mjs --base-url http://127.0.0.1:8080
 *
 * Requires: npx playwright install chromium (done automatically if missing)
 */

import { parseArgs } from "node:util";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { execSync } from "node:child_process";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ASSETS_DIR = resolve(__dirname, "../../docs/assets");

const PAGES = {
  dashboard: {
    path: "/",
    file: "overview.png",
    label: "Dashboard overview",
    width: 1280,
    height: 800,
  },
  zones: {
    path: "/zones",
    file: "zones.png",
    label: "Local zone management",
    width: 1280,
    height: 800,
  },
  "dashboard-mobile": {
    path: "/",
    file: "mobile.png",
    label: "Dashboard (mobile)",
    width: 390,
    height: 844,
  },
  "zones-mobile": {
    path: "/zones",
    file: "zones-mobile.png",
    label: "Zone management (mobile)",
    width: 390,
    height: 844,
  },
};

const { values } = parseArgs({
  options: {
    "base-url": { type: "string", default: "http://127.0.0.1:8080" },
    pages: { type: "string", multiple: true },
    headless: { type: "boolean", default: true },
    timeout: { type: "string", default: "30000" },
    help: { type: "boolean", short: "h" },
  },
  strict: false,
});

if (values.help) {
  console.log(`
Velora DNS Screenshot Capture

Usage:
  node scripts/screenshot.mjs [options]

Options:
  --base-url <url>    Base URL of the running Velora DNS instance
                      (default: http://127.0.0.1:8080)
  --pages <name>      Capture specific pages only (can be repeated)
                      Available: ${Object.keys(PAGES).join(", ")}
  --headless          Run browser in headless mode (default: true)
  --no-headless       Run browser in visible mode
  --timeout <ms>      Navigation timeout in ms (default: 30000)
  -h, --help          Show this help

Examples:
  node scripts/screenshot.mjs
  node scripts/screenshot.mjs --pages dashboard zones
  node scripts/screenshot.mjs --base-url http://192.168.1.10:8080 --no-headless
`);
  process.exit(0);
}

async function ensurePlaywright() {
  try {
    return await import("playwright");
  } catch {
    console.log("Playwright not found. Installing...");
    execSync("npm install --no-save playwright", {
      cwd: resolve(__dirname, ".."),
      stdio: "inherit",
    });
    execSync("npx playwright install chromium", {
      cwd: resolve(__dirname, ".."),
      stdio: "inherit",
    });
    return await import("playwright");
  }
}

const baseUrl = values["base-url"];
const timeout = parseInt(values.timeout, 10);
const headless = values.headless;
const selectedPages = values.pages?.length
  ? values.pages
  : Object.keys(PAGES);

async function capturePage(browser, pageName) {
  const config = PAGES[pageName];
  if (!config) {
    console.error(`Unknown page: ${pageName}`);
    return false;
  }

  const context = await browser.newContext({
    viewport: { width: config.width, height: config.height },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();

  try {
    await page.goto(`${baseUrl}${config.path}`, {
      waitUntil: "networkidle",
      timeout,
    });
    await page.waitForTimeout(2000);

    const outputPath = resolve(ASSETS_DIR, config.file);
    await page.screenshot({ path: outputPath, fullPage: false });
    console.log(`  ${config.label} -> ${config.file}`);
    return true;
  } catch (err) {
    console.error(`  Failed to capture ${pageName}: ${err.message}`);
    return false;
  } finally {
    await context.close();
  }
}

async function main() {
  console.log(`Capturing screenshots from ${baseUrl}\n`);

  const { chromium } = await ensurePlaywright();
  const browser = await chromium.launch({ headless });

  const results = [];
  for (const pageName of selectedPages) {
    const ok = await capturePage(browser, pageName);
    results.push({ page: pageName, ok });
  }

  await browser.close();

  console.log("\nDone.");
  const failed = results.filter((r) => !r.ok);
  if (failed.length) {
    console.error(`\nFailed: ${failed.map((r) => r.page).join(", ")}`);
    process.exit(1);
  }
}

main();
