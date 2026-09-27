#!/usr/bin/env node
// Starts a fresh Velora DNS installation for the browser tests: temporary
// database, built web UI, fixed local ports and a bootstrap admin. The
// server log is written to e2e-results/server.log for CI artifacts.
import { spawn } from "node:child_process";
import { createWriteStream, mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const web = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const root = resolve(web, "..");
const httpPort = process.env.VELORA_E2E_HTTP_PORT ?? "18080";
const dnsPort = process.env.VELORA_E2E_DNS_PORT ?? "15353";
const dir = mkdtempSync(join(tmpdir(), "velora-e2e-"));
const config = join(dir, "config.yaml");
writeFileSync(config, `dns:
  listen: ['127.0.0.1:${dnsPort}']
  upstreams: ['127.0.0.1:9']
  allowed_clients: ['127.0.0.0/8', '::1/128']
http:
  listen: '127.0.0.1:${httpPort}'
  web_dir: '${join(web, "dist")}'
  allowed_hosts: ['127.0.0.1', 'localhost']
database_path: '${join(dir, "velora.db")}'
log_level: info
query_log:
  enabled: true
  queue_size: 1024
  retention: 1h
  max_rows: 10000
filtering:
  block_mode: NXDOMAIN
  blocklist: ['ads.e2e.test']
`);
mkdirSync(join(web, "e2e-results"), { recursive: true });
const log = createWriteStream(join(web, "e2e-results", "server.log"));
const [command, args] = process.env.VELORA_BIN ? [process.env.VELORA_BIN, []] : ["go", ["run", "./cmd/server"]];
const child = spawn(command, [...args, "-config", config], {
  cwd: root,
  env: { ...process.env, VELORA_BOOTSTRAP_USERNAME: "admin", VELORA_BOOTSTRAP_PASSWORD: "e2e admin password", VELORA_UPDATER_SOCKET: join(dir, "no-updater.sock") },
  stdio: ["ignore", "pipe", "pipe"],
});
child.stdout.pipe(log);
child.stderr.pipe(log);
child.stderr.pipe(process.stderr);
const stop = () => child.kill("SIGTERM");
process.on("SIGTERM", stop);
process.on("SIGINT", stop);
child.on("exit", (code) => process.exit(code ?? 0));
