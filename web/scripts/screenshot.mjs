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
  "dashboard": {
    path: "/",
    file: "overview.png",
    label: "Dashboard overview",
    width: 1440,
    height: 1000,
  },
  "dashboard-dark": {
    path: "/",
    file: "overview-dark.png",
    label: "Dashboard overview (dark theme)",
    width: 1440,
    height: 1000,
    theme: "dark",
  },
  "dashboard-mobile": {
    path: "/",
    file: "overview-mobile.png",
    label: "Dashboard overview (mobile)",
    width: 390,
    height: 844,
  },
  "navigation-mobile": {
    path: "/",
    file: "navigation-mobile.png",
    label: "Navigation drawer (mobile)",
    width: 390,
    height: 844,
    openNav: true,
  },
  "queries": {
    path: "/queries",
    file: "query-log.png",
    label: "Query log",
    width: 1440,
    height: 1000,
  },
  "events": {
    path: "/events",
    file: "events.png",
    label: "Event center",
    width: 1440,
    height: 1000,
  },
  "zones": {
    path: "/zones",
    file: "zones.png",
    label: "Local zone management",
    width: 1440,
    height: 1000,
  },
  "rewrites": {
    path: "/rewrites",
    file: "rewrites.png",
    label: "DNS rewrites",
    width: 1440,
    height: 1000,
  },
  "forwarding": {
    path: "/forwarding",
    file: "forwarding.png",
    label: "Conditional forwarding",
    width: 1440,
    height: 1000,
  },
  "blocklists": {
    path: "/blocklists",
    file: "blocklists.png",
    label: "Blocklists",
    width: 1440,
    height: 1000,
  },
  "cache": {
    path: "/cache",
    file: "cache.png",
    label: "DNS cache",
    width: 1440,
    height: 1000,
  },
  "clients": {
    path: "/clients",
    file: "clients.png",
    label: "Clients and devices",
    width: 1440,
    height: 1000,
  },
  "dhcp": {
    path: "/dhcp",
    file: "dhcp.png",
    label: "DHCP server",
    width: 1440,
    height: 1000,
  },
  "cluster": {
    path: "/cluster",
    file: "cluster.png",
    label: "Cluster overview",
    width: 1440,
    height: 1000,
  },
  "settings": {
    path: "/settings",
    file: "settings.png",
    label: "Settings",
    width: 1440,
    height: 1000,
    fullPage: true,
  },
  "updates": {
    path: "/updates",
    file: "update-center.png",
    label: "Update Center",
    width: 1440,
    height: 1000,
  },
  "backup": {
    path: "/backup",
    file: "backup.png",
    label: "Backup and restore",
    width: 1440,
    height: 1000,
  },
  "diagnostics": {
    path: "/diagnostics",
    file: "diagnostics.png",
    label: "Diagnostics",
    width: 1440,
    height: 1000,
  },
  "audit": {
    path: "/audit",
    file: "audit-log.png",
    label: "Audit log",
    width: 1440,
    height: 1000,
  },
  "login": {
    path: "/",
    file: "sign-in.png",
    label: "Sign-in screen",
    width: 1440,
    height: 1000,
    signedOut: true,
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
  { id: 4, occurred_at: "2026-09-21T12:04:00Z", client_ip: "192.0.2.45", client_name: "Living Room TV", domain: "docs.example", type: "A", rcode: "NOERROR", duration: 1200000, source: "cache", upstream: "", cache_hit: true },
  { id: 3, occurred_at: "2026-09-21T12:03:42Z", client_ip: "192.0.2.21", client_name: "Home Assistant", domain: "router.home.arpa", type: "A", rcode: "NOERROR", duration: 340000, source: "local", upstream: "", cache_hit: false },
  { id: 2, occurred_at: "2026-09-21T12:03:18Z", client_ip: "192.0.2.45", client_name: "Living Room TV", domain: "telemetry.example", type: "AAAA", rcode: "NXDOMAIN", duration: 410000, source: "blocked", upstream: "", cache_hit: false },
  { id: 1, occurred_at: "2026-09-21T12:02:51Z", client_ip: "192.0.2.33", domain: "example.org", type: "MX", rcode: "NOERROR", duration: 18300000, source: "upstream", upstream: "9.9.9.9:53", cache_hit: false },
];

const hoursFromNow = (hours) => new Date(Date.now() + hours * 3600000).toISOString();

const demoEvents = [
  { id: 3, severity: "warning", title: "Blocklist refresh failed", message: "Community hosts returned HTTP 503. The previous copy stays active.", occurred_at: "2026-09-21T11:00:00Z", read: false, repeat_count: 2, link: "/blocklists" },
  { id: 2, severity: "info", title: "Update installed", message: "Velora DNS 0.1.0-beta.10 passed its readiness check.", occurred_at: "2026-09-20T20:52:46Z", read: true, repeat_count: 1, link: "/updates" },
  { id: 1, severity: "critical", title: "Upstream unavailable", message: "9.9.9.9:53 failed five consecutive health checks and recovered.", occurred_at: "2026-09-19T08:14:00Z", read: true, repeat_count: 1 },
];

const demoBlocklists = [
  { id: 1, name: "Community hosts", url: "https://example.org/hosts.txt", enabled: true, domain_count: 84213, last_updated_at: "2026-09-21T10:00:00Z", last_error: "", update_interval: 86400, last_attempt_at: "2026-09-21T10:00:00Z", consecutive_failures: 0, next_update_at: hoursFromNow(22) },
  { id: 2, name: "Local overrides", url: "", enabled: true, domain_count: 12, last_updated_at: "2026-09-18T09:30:00Z", last_error: "", update_interval: 0, consecutive_failures: 0, next_update_at: null },
  { id: 3, name: "Tracker list", url: "https://example.net/trackers.txt", enabled: true, domain_count: 5120, last_updated_at: "2026-09-20T06:00:00Z", last_error: "Download or list validation failed; previous domains retained.", update_interval: 21600, last_attempt_at: "2026-09-21T11:40:00Z", consecutive_failures: 2, next_update_at: hoursFromNow(0.25) },
];

const demoClients = {
  clients: [
    { id: 1, name: "Living Room TV", addresses: ["192.0.2.45"], group: "Media", description: "", enabled: true, activity: { queries: 5400, last_seen: hoursFromNow(-0.05) } },
    { id: 2, name: "Home Assistant", addresses: ["192.0.2.21", "2001:db8::21"], group: "Automation", description: "Raspberry Pi 5", enabled: true, activity: { queries: 3720, last_seen: hoursFromNow(-0.01) } },
    { id: 3, name: "Guest network", addresses: ["198.51.100.0/24"], group: "Networks", description: "Isolated VLAN", enabled: true, activity: { queries: 612, last_seen: hoursFromNow(-2) } },
    { id: 4, name: "Old laptop", addresses: ["192.0.2.90"], group: "", description: "", enabled: false, activity: { queries: 0, last_seen: null } },
  ],
  unnamed: [{ client_ip: "192.0.2.33", queries: 241, last_seen: hoursFromNow(-0.3) }, { client_ip: "192.0.2.61", queries: 57, last_seen: hoursFromNow(-5) }],
};

const demoRewrites = [
  { id: 1, name: "nas.home.arpa", type: "A", value: "192.0.2.20", enabled: true, description: "Replaces the zone record during migration", overrides_zone: "home.arpa" },
  { id: 2, name: "nas.home.arpa", type: "AAAA", value: "2001:db8::20", enabled: true, description: "", overrides_zone: "home.arpa" },
  { id: 3, name: "*.dev.lab.test", type: "A", value: "192.0.2.30", enabled: true, description: "Preview environments" },
  { id: 4, name: "grafana.lab.test", type: "CNAME", value: "monitoring.lab.test", enabled: true, description: "" },
  { id: 5, name: "telemetry.example", type: "A", value: "192.0.2.99", enabled: false, description: "", blocked_by: "blocklist" },
];

const demoForwarding = [
  { id: 1, domain: "corp.example", upstreams: ["198.51.100.10:53", "198.51.100.11:53"], enabled: true, description: "Office directory servers", health: [{ address: "198.51.100.10:53", state: "healthy", consecutive_failures: 0, latency_milliseconds: 3.8 }, { address: "198.51.100.11:53", state: "degraded", consecutive_failures: 1, latency_milliseconds: 0 }] },
  { id: 2, domain: "lab.corp.example", upstreams: ["192.0.2.53:5353"], enabled: true, description: "Lab resolver", health: [{ address: "192.0.2.53:5353", state: "healthy", consecutive_failures: 0, latency_milliseconds: 1.2 }] },
  { id: 3, domain: "home.arpa", upstreams: ["192.0.2.1:53"], enabled: false, description: "", health: [] },
];

const demoCacheEntries = [
  { name: "docs.example.", type: "A", remaining_ttl: 240, rcode: "NOERROR", answers: ["docs.example. 300 IN A 192.0.2.10"] },
  { name: "example.org.", type: "AAAA", remaining_ttl: 61, rcode: "NOERROR", answers: ["example.org. 300 IN AAAA 2001:db8::10"] },
  { name: "mail.example.org.", type: "MX", remaining_ttl: 1810, rcode: "NOERROR", answers: ["example.org. 3600 IN MX 10 mail.example.org."] },
  { name: "missing.example.", type: "A", remaining_ttl: 30, rcode: "NXDOMAIN", answers: [] },
];

const demoLeases = [
  { id: 1, pool_id: 1, mac_address: "00:00:5e:00:53:01", ip_address: "198.51.100.50", hostname: "laptop", expires_at: hoursFromNow(19), status: "active" },
  { id: 2, pool_id: 1, mac_address: "00:00:5e:00:53:02", ip_address: "198.51.100.51", hostname: "printer", expires_at: hoursFromNow(6), status: "active" },
];

const demoNodes = [
  { id: "a1b2c3d4e5f60718", name: "velora-1", address: "192.0.2.2:8080", status: "healthy", version: "0.1.0-beta.11", last_seen_at: hoursFromNow(0) },
  { id: "f6e5d4c3b2a10987", name: "velora-2", address: "192.0.2.3:8080", status: "healthy", version: "0.1.0-beta.11", last_seen_at: hoursFromNow(0) },
];

const demoAudit = [
  { id: 3, occurred_at: "2026-09-21T12:00:00Z", actor: "demo-admin", role: "admin", action: "zone.record.update", target: "/api/v1/zones/1/records/2", result: "success", status_code: 200 },
  { id: 2, occurred_at: "2026-09-21T11:42:00Z", actor: "demo-admin", role: "admin", action: "blocklist.update", target: "/api/v1/blocklists/1/update", result: "success", status_code: 202 },
  { id: 1, occurred_at: "2026-09-21T10:05:00Z", actor: "demo-viewer", role: "viewer", action: "cache.flush", target: "/api/v1/cache", result: "failure", status_code: 403 },
];

function demoResponse(pathname) {
  if (pathname === "/api/v1/auth/me") return { username: "demo-admin", role: "admin", csrf_token: "screenshot-only" };
  if (pathname === "/api/v1/preferences") return { language: "en", theme: "light" };
  if (pathname === "/api/v1/status") return { ready: true, uptime_seconds: 47232, dns_listen: ["127.0.0.1:53"], version: { version: "0.1.0-beta.11", commit: "demo", built: "2026-09-21T12:00:00Z" }, capabilities: [] };
  if (pathname === "/api/v1/stats") return { queries_total: 18472, blocked_queries: 629, queries_per_second: 12.4, cache_hit_rate: 0.847 };
  if (pathname === "/api/v1/cache") return { entries: 1842, capacity: 10000, hits: 15646, misses: 2826 };
  if (pathname === "/api/v1/config") return { query_log: { enabled: true, retention: 604800000000000, max_rows: 100000, queue_size: 1024 }, dns: { listen: ["127.0.0.1:53"], upstreams: ["1.1.1.1:53", "9.9.9.9:53"], allowed_clients: ["127.0.0.0/8"], timeout: 2000000000, retries: 1, max_concurrent: 256 }, cache: { max_entries: 10000, upstream_ttl: 86400 }, http: { listen: "127.0.0.1:8080", web_dir: "web/dist", allowed_hosts: ["localhost"] }, log_level: "info" };
  if (pathname === "/api/v1/query-stats") return { window_start: "2026-09-20T12:00:00Z", window_end: "2026-09-21T12:00:00Z", total: 18472, blocked: 629, top_domains: [{ value: "docs.example", count: 1284 }, { value: "example.org", count: 914 }], top_clients: [{ value: "192.0.2.45", count: 5400, name: "Living Room TV" }, { value: "192.0.2.21", count: 3720, name: "Home Assistant" }] };
  if (pathname === "/api/v1/zones") return demoZones;
  if (pathname === "/api/v1/queries") return demoQueries;
  if (pathname === "/api/v1/update/status") return { state: "completed", installed: "0.1.0-beta.10", last_completed: "2026-09-20T20:52:46Z", updating: false };
  if (pathname === "/api/v1/update/history") return [{ id: "demo-update", started_at: "2026-09-20T20:50:02Z", completed_at: "2026-09-20T20:52:46Z", from_version: "0.1.0-beta.9", to_version: "0.1.0-beta.10", channel: "beta", state: "completed", readiness_ok: true, rollback_used: false, deployment_mode: "compose" }];
  if (pathname === "/api/v1/upstreams/health") return [{ address: "1.1.1.1:53", state: "healthy", consecutive_failures: 0, latency_milliseconds: 12.4 }, { address: "9.9.9.9:53", state: "healthy", consecutive_failures: 0, latency_milliseconds: 18.1 }];
  if (pathname === "/api/v1/events") return { unread_count: 1, events: demoEvents };
  if (pathname === "/api/v1/blocklists") return demoBlocklists;
  if (pathname === "/api/v1/forwarding") return demoForwarding;
  if (pathname === "/api/v1/rewrites") return demoRewrites;
  if (pathname === "/api/v1/clients") return demoClients;
  if (pathname === "/api/v1/cache/entries") return { total: demoCacheEntries.length, entries: demoCacheEntries };
  if (pathname === "/api/v1/settings/rate-limit") return { enabled: true, global_qps: 1000, client_qps: 50, rate_limit_burst: 100 };
  if (pathname === "/api/v1/settings/rate-limit/status") return { rejected_total: 14, last_rejected_at: "2026-09-21T11:30:00Z" };
  if (pathname === "/api/v1/backup/status") return { supported: true, last_backup_time: "2026-09-21T02:00:00Z", backup_age: "10h", verification_state: "verified", database_path: "/var/lib/velora/velora.db" };
  if (pathname === "/api/v1/dhcp/pools") return [{ id: 1, name: "lan", interface: "eth0", subnet: "198.51.100.0/24", gateway: "198.51.100.1", dns_servers: ["198.51.100.2"], lease_seconds: 86400, enabled: true }];
  if (pathname === "/api/v1/dhcp/leases") return demoLeases;
  if (pathname === "/api/v1/cluster/nodes") return demoNodes;
  if (pathname === "/api/v1/cluster/config-versions") return [{ version: 7, config_hash: "9f8e7d6c5b4a3928", applied_by: "demo-admin", applied_at: "2026-09-21T09:00:00Z" }, { version: 6, config_hash: "1a2b3c4d5e6f7081", applied_by: "demo-admin", applied_at: "2026-09-20T15:12:00Z" }];
  if (pathname === "/api/v1/diagnostics") return { generated_at: "2026-09-21T12:00:00Z", version: { version: "0.1.0-beta.11", commit: "demo", built: "2026-09-21T12:00:00Z" }, os: "linux", architecture: "amd64", uptime_seconds: 47232, state: "healthy", components: [{ name: "dns_listener", state: "healthy", detail: "127.0.0.1:53 udp/tcp" }, { name: "upstreams", state: "healthy", detail: "2 of 2 reachable" }, { name: "database", state: "healthy", detail: "SQLite schema up to date" }, { name: "query_log", state: "healthy", detail: "Queue drained" }], upstream_count: 2, query_log_enabled: true };
  if (pathname === "/api/v1/audit") return demoAudit;
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
    if (config.signedOut && pathname === "/api/v1/auth/me") {
      await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: { message: "Sign in required" } }) });
      return;
    }
    const data = pathname === "/api/v1/preferences"
      ? { language: "en", theme: config.theme ?? "light" }
      : demoResponse(pathname);
    if (data === null) {
      await route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ error: { message: `No screenshot fixture for ${pathname}` } }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data }) });
  });
  await page.addInitScript((theme) => {
    localStorage.setItem("velora_language", "en");
    localStorage.setItem("velora_theme", theme);
  }, config.theme ?? "light");

  try {
    await page.goto(`${baseUrl}${config.path}`, {
      waitUntil: "networkidle",
      timeout,
    });
    await page.locator("main").waitFor();
    await page.waitForTimeout(500);
    if (config.openNav) {
      await page.click(".menu-button");
      await page.waitForTimeout(400);
    }

    const outputPath = resolve(ASSETS_DIR, config.file);
    await page.screenshot({ path: outputPath, fullPage: Boolean(config.fullPage) });
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
