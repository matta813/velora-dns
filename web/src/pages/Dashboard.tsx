import { Activity, ArrowUpRight, Database, Timer, Zap } from "lucide-react";
import type { Snapshot } from "../api";
import { Stat } from "../components/Stat";
const number = (v: number) => new Intl.NumberFormat("en").format(v);
export function Dashboard({
  data,
  history,
}: {
  data: Snapshot;
  history: number[];
}) {
  const max = Math.max(1, ...history);
  const points = history
    .map(
      (v, i) =>
        `${(i / Math.max(1, history.length - 1)) * 600},${125 - (v / max) * 100}`,
    )
    .join(" ");
  return (
    <>
      <div className="stats">
        <Stat
          label="Total queries"
          value={number(data.stats.queries_total)}
          note="Since this server started"
          icon={<Activity size={17} />}
        />
        <Stat
          label="Queries / second"
          value={data.stats.queries_per_second.toFixed(2)}
          note="Rolling 60-second average"
          icon={<Zap size={17} />}
        />
        <Stat
          label="Cache hit rate"
          value={`${(data.stats.cache_hit_rate * 100).toFixed(1)}%`}
          note={`${number(data.cache.hits)} answers served from memory`}
          icon={<Database size={17} />}
        />
        <Stat
          label="Uptime"
          value={`${Math.floor(data.status.uptime_seconds / 3600)}h ${Math.floor(data.status.uptime_seconds / 60) % 60}m`}
          note="Current process lifetime"
          icon={<Timer size={17} />}
        />
      </div>
      <div className="overview-grid">
        <section className="panel traffic">
          <div className="panel-heading">
            <div>
              <h2>Query activity</h2>
              <p>Live samples from your resolver</p>
            </div>
            <span className="subtle-badge">5s refresh</span>
          </div>
          <div className="chart-label">
            <strong>{data.stats.queries_per_second.toFixed(2)}</strong>
            <span>queries / sec</span>
          </div>
          <svg
            className="chart"
            viewBox="0 0 600 145"
            role="img"
            aria-label="Query rate sampled during this browser session"
          >
            <path d="M0 25H600 M0 75H600 M0 125H600" className="chart-grid" />
            <polyline points={points} className="chart-line" />
          </svg>
          <div className="chart-footer">
            <span>Session samples · {history.length} / 30</span>
            <span>Now</span>
          </div>
        </section>
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>Resolver pipeline</h2>
              <p>How a query reaches its answer</p>
            </div>
          </div>
          <ol className="pipeline">
            <li>
              <span>01</span>
              <div>
                <strong>Client access</strong>
                <p>Allowed network check</p>
              </div>
              <i className="dot" />
            </li>
            <li>
              <span>02</span>
              <div>
                <strong>Memory cache</strong>
                <p>
                  {number(data.cache.entries)} / {number(data.cache.capacity)}{" "}
                  entries
                </p>
              </div>
              <i className="dot" />
            </li>
            <li>
              <span>03</span>
              <div>
                <strong>Upstream forwarding</strong>
                <p>{data.config.dns.upstreams.length} configured resolvers</p>
              </div>
              <ArrowUpRight size={16} />
            </li>
          </ol>
        </section>
      </div>
      <div className="overview-grid">
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>Upstream resolvers</h2>
              <p>Ordered failover with TCP fallback</p>
            </div>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Endpoint</th>
                  <th>Priority</th>
                  <th>Attempt timeout</th>
                </tr>
              </thead>
              <tbody>
                {data.config.dns.upstreams.map((upstream, i) => (
                  <tr key={upstream}>
                    <td>
                      <span className="endpoint-mark">↗</span>
                      <code>{upstream}</code>
                    </td>
                    <td>{i === 0 ? "Primary" : `Fallback ${i}`}</td>
                    <td>{data.config.dns.timeout / 1e9}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="panel-footnote">
            Configuration shown. Reachability is evaluated on each query.
          </p>
        </section>
        <section className="foundation-card">
          <span className="eyebrow">BUILDING THE FOUNDATION</span>
          <h2>
            Your DNS.
            <br />
            Under your control.
          </h2>
          <p>
            Forwarding, a bounded TTL cache, and operational visibility. Local
            zones, filtering, and query history are the next steps.
          </p>
          <a href="https://github.com/matta813/velora-dns/issues">
            Explore the roadmap <ArrowUpRight size={16} />
          </a>
        </section>
      </div>
    </>
  );
}
