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
    width: 1440,
    height: 1000,
  },
  queries: {
    path: "/queries",
    file: "query-log.png",
    label: "Query log",
    width: 1440,
    height: 1000,
  },
  updates: {
    path: "/updates",
    file: "update-center.png",
    label: "Update Center",
    width: 1440,
    height: 1000,
  },
  zones: {
    path: "/zones",
    file: "zones.png",
    label: "Local zone management",
    width: 1440,
    height: 1000,
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

const demoZones = [
  {
    id: 1,
    name: "home.arpa",
    primary_ns: "ns.home.arpa",
    contact: "hostmaster.home.arpa",
    revision: 4,
    records: [
      { id: 1, name: "router.home.arpa", type: "A", ttl: 300, value: "192.0.2.1", priority: 0 },
      { id: 2, name: "nas.home.arpa", type: "A", ttl: 300, value: "192.0.2.20", priority: 0 },
      { id: 3, name: "docs.home.arpa", type: "CNAME", ttl: 300, value: "nas.home.arpa.", priority: 0 },
    ],
  },
  {
    id: 2,
    name: "lab.test",
    primary_ns: "ns.lab.test",
    contact: "hostmaster.lab.test",
    revision: 2,
    records: [
      { id: 4, name: "resolver.lab.test", type: "AAAA", ttl: 600, value: "2001:db8::53", priority: 0 },
    ],
  },
];

const demoQueries = [
  { id: 4, occurred_at: "2026-09-21T12:04:00Z", client_ip: "192.0.2.45", domain: "docs.example", type: "A", rcode: "NOERROR", duration: 1200000, source: "cache", upstream: "", cache_hit: true },
  { id: 3, occurred_at: "2026-09-21T12:03:42Z", client_ip: "192.0.2.21", domain: "router.home.arpa", type: "A", rcode: "NOERROR", duration: 340000, source: "local", upstream: "", cache_hit: false },
  { id: 2, occurred_at: "2026-09-21T12:03:18Z", client_ip: "192.0.2.45", domain: "telemetry.example", type: "AAAA", rcode: "NXDOMAIN", duration: 410000, source: "blocked", upstream: "", cache_hit: false },
  { id: 1, occurred_at: "2026-09-21T12:02:51Z", client_ip: "192.0.2.33", domain: "example.org", type: "MX", rcode: "NOERROR", duration: 18300000, source: "upstream", upstream: "9.9.9.9:53", cache_hit: false },
];

function demoResponse(pathname) {
  if (pathname === "/api/v1/auth/me") return { username: "demo-admin", role: "admin", csrf_token: "screenshot-only" };
  if (pathname === "/api/v1/preferences") return { language: "en", theme: "light" };
  if (pathname === "/api/v1/status") return { ready: true, uptime_seconds: 47232, dns_listen: ["127.0.0.1:53"], version: { version: "0.1.0-beta.11", commit: "demo", built: "2026-09-21T12:00:00Z" }, capabilities: [] };
  if (pathname === "/api/v1/stats") return { queries_total: 18472, blocked_queries: 629, queries_per_second: 12.4, cache_hit_rate: 0.847 };
  if (pathname === "/api/v1/cache") return { entries: 1842, capacity: 10000, hits: 15646, misses: 2826 };
  if (pathname === "/api/v1/config") return { query_log: { enabled: true, retention: 604800000000000, max_rows: 100000, queue_size: 1024 }, dns: { listen: ["127.0.0.1:53"], upstreams: ["1.1.1.1:53", "9.9.9.9:53"], allowed_clients: ["127.0.0.0/8"], timeout: 2000000000, retries: 1, max_concurrent: 256 }, cache: { max_entries: 10000, upstream_ttl: 86400 }, http: { listen: "127.0.0.1:8080", web_dir: "web/dist", allowed_hosts: ["localhost"] }, log_level: "info" };
  if (pathname === "/api/v1/query-stats") return { window_start: "2026-09-20T12:00:00Z", window_end: "2026-09-21T12:00:00Z", total: 18472, blocked: 629, top_domains: [{ value: "docs.example", count: 1284 }, { value: "example.org", count: 914 }], top_clients: [{ value: "192.0.2.45", count: 5400 }, { value: "192.0.2.21", count: 3720 }] };
  if (pathname === "/api/v1/zones") return demoZones;
  if (pathname === "/api/v1/queries") return demoQueries;
  if (pathname === "/api/v1/update/status") return { state: "completed", installed: "0.1.0-beta.10", last_completed: "2026-09-20T20:52:46Z", updating: false };
  if (pathname === "/api/v1/update/history") return [{ id: "demo-update", started_at: "2026-09-20T20:50:02Z", completed_at: "2026-09-20T20:52:46Z", from_version: "0.1.0-beta.9", to_version: "0.1.0-beta.10", channel: "beta", state: "completed", readiness_ok: true, rollback_used: false, deployment_mode: "compose" }];
  if (pathname === "/api/v1/update/check") return { installed_version: "0.1.0-beta.10", latest_version: "0.1.0-beta.11", update_available: true, channel: "beta", release_date: "2026-09-21T17:30:36Z", release_notes: "UI-driven updates, safer rollback, and refreshed operations tooling.", architecture: "linux/amd64", download_size: 24117248 };
  return null;
}

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
    locale: "en-US",
    timezoneId: "UTC",
  });
  const page = await context.newPage();

  await page.route("**/api/v1/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    const data = demoResponse(pathname);
    if (data === null) {
      await route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ error: { message: `No screenshot fixture for ${pathname}` } }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data }) });
  });
  await page.addInitScript(() => {
    localStorage.setItem("velora_language", "en");
    localStorage.setItem("velora_theme", "light");
  });

  try {
    await page.goto(`${baseUrl}${config.path}`, {
      waitUntil: "networkidle",
      timeout,
    });
    await page.locator("main").waitFor();
    await page.waitForTimeout(500);

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
