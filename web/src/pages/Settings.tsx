import type { Snapshot } from "../api";
export function Settings({ data }: { data: Snapshot }) {
  const rows: [string, string][] = [
    ["DNS listeners", data.status.dns_listen.join(", ")],
    ["Allowed client networks", data.config.dns.allowed_clients.join(", ")],
    ["Upstreams", data.config.dns.upstreams.join(", ")],
    ["Attempt timeout", `${data.config.dns.timeout / 1e9} seconds`],
    ["Retries after first round", String(data.config.dns.retries)],
    ["Maximum concurrent queries", String(data.config.dns.max_concurrent)],
    ["Cache capacity", String(data.cache.capacity)],
    ["Log level", data.config.log_level],
    ["Version", data.status.version.version],
  ];
  return (
    <section className="panel padded">
      <h2>Server configuration</h2>
      <p>
        Read-only in this foundation. Update YAML or environment variables and
        restart the server to apply changes.
      </p>
      <dl className="settings-list">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>
              <code>{value}</code>
            </dd>
          </div>
        ))}
      </dl>
      <div className="notice">
        Management authentication is planned. Keep this interface on a trusted
        local network.
      </div>
    </section>
  );
}
