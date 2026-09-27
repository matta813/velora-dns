// Demo API responses shared by the screenshot, accessibility audit and
// end-to-end scripts. They use documentation address ranges only.

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

export const hoursFromNow = (hours) => new Date(Date.now() + hours * 3600000).toISOString();

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

const demoPolicies = [
  { id: 1, client_id: 1, mode: "custom", blocklists: [1, 3], allow: ["streaming.example"], block: ["ads.tv.example", "telemetry.tv.example"], enabled: true },
  { id: 2, client_id: 2, mode: "disabled", blocklists: [], allow: [], block: [], enabled: true },
  { id: 3, client_id: 3, mode: "custom", blocklists: [1, 2, 3], allow: [], block: ["games.example"], enabled: true },
  { id: 4, client_id: 4, mode: "default", blocklists: [], allow: [], block: [], enabled: false },
];

const demoWebhooks = [
  { id: 1, name: "Home Assistant", url: "http://192.0.2.21:8123/api/webhook/velora-dns", events: ["upstream.unavailable", "upstream.recovered"], min_severity: "info", allow_private: true, enabled: true, has_token: false, last_delivery_at: hoursFromNow(-1.5), last_status: "HTTP 200", last_error: "", consecutive_failures: 0 },
  { id: 2, name: "On-call pager", url: "https://alerts.example.com/hooks/velora", events: [], min_severity: "warning", allow_private: false, enabled: true, has_token: true, last_delivery_at: hoursFromNow(-6), last_status: "HTTP 202", last_error: "", consecutive_failures: 0 },
  { id: 3, name: "Team chat", url: "https://chat.example.net/hooks/ops", events: ["blocklist.refresh_failed", "backup.failed"], min_severity: "info", allow_private: false, enabled: false, has_token: true, last_delivery_at: hoursFromNow(-30), last_status: "failed", last_error: "endpoint returned HTTP 503", consecutive_failures: 4 },
];

// A plausible home-network day: quiet at night, busy in the evening.
function demoAnalytics() {
  const end = new Date();
  end.setUTCMinutes(0, 0, 0);
  const series = Array.from({ length: 24 }, (_, i) => {
    const start = new Date(end.getTime() - (23 - i) * 3600000);
    const hour = start.getHours();
    const load = 0.25 + 0.75 * Math.max(0, Math.sin(((hour - 6) / 24) * Math.PI * 2)) + (hour >= 18 && hour <= 22 ? 0.5 : 0);
    const total = Math.round(420 + 900 * load + ((i * 37) % 60));
    const blocked = Math.round(total * (0.028 + ((i * 7) % 5) / 400));
    return { start: start.toISOString(), total, blocked, cached: Math.round(total * 0.83), failed: i === 15 ? 6 : i % 7 === 0 ? 1 : 0, average_ms: 14 + ((i * 13) % 9) + (i === 15 ? 38 : 0) };
  });
  const sum = (key) => series.reduce((acc, bucket) => acc + bucket[key], 0);
  const total = sum("total");
  return {
    range: "24h", window_start: series[0].start, window_end: end.toISOString(), bucket_seconds: 3600, history_start: hoursFromNow(-24 * 6),
    totals: { start: series[0].start, total, blocked: sum("blocked"), cached: sum("cached"), failed: sum("failed"), average_ms: 18.6 },
    series,
    query_types: [{ value: "A", count: Math.round(total * 0.52) }, { value: "AAAA", count: Math.round(total * 0.31) }, { value: "HTTPS", count: Math.round(total * 0.1) }, { value: "PTR", count: Math.round(total * 0.04) }, { value: "TXT", count: Math.round(total * 0.02) }, { value: "SRV", count: Math.round(total * 0.01) }],
    response_codes: [{ value: "NOERROR", count: Math.round(total * 0.93) }, { value: "NXDOMAIN", count: Math.round(total * 0.065) }, { value: "SERVFAIL", count: sum("failed") }],
    sources: [{ value: "cache", count: sum("cached") }, { value: "upstream", count: Math.round(total * 0.11) }, { value: "blocked", count: sum("blocked") }, { value: "local", count: Math.round(total * 0.025) }],
    upstreams: [{ address: "1.1.1.1:53", queries: Math.round(total * 0.08), failed: 9, average_ms: 16.2 }, { address: "9.9.9.9:53", queries: Math.round(total * 0.03), failed: 2, average_ms: 24.9 }],
    top_blocked: [{ value: "telemetry.example.", count: 412 }, { value: "ads.example.net.", count: 318 }, { value: "tracker.example.org.", count: 201 }, { value: "metrics.tv.example.", count: 96 }],
  };
}

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

export function demoResponse(pathname) {
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
  if (pathname === "/api/v1/policies") return demoPolicies;
  if (pathname === "/api/v1/analytics") return demoAnalytics();
  if (pathname === "/api/v1/webhooks") return demoWebhooks;
  if (pathname === "/api/v1/webhooks/event-types") return ["upstream.unavailable", "upstream.recovered", "blocklist.refresh_failed", "blocklist.refresh_recovered", "backup.created", "backup.failed", "config.rollback"];
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
