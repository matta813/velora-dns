import { useEffect, useState } from "react";
import { Activity, ArrowUpRight, Database, Timer, Zap } from "lucide-react";
import { request, type QuerySummary, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { OnboardingChecklist } from "../components/OnboardingChecklist";
import { useI18n } from "../i18n-context";
const number = (v: number) => new Intl.NumberFormat("en").format(v);
export function Dashboard({
  data,
  history,
  queryLoggingEnabled,
}: {
  data: Snapshot;
  history: number[];
  queryLoggingEnabled: boolean;
}) {
  const { t } = useI18n();
  const [querySummary, setQuerySummary] = useState<QuerySummary | null>(null);
  const [summaryError, setSummaryError] = useState("");
  useEffect(() => {
    if (!queryLoggingEnabled) {
      return;
    }
    const controller = new AbortController();
    void request<QuerySummary>("/api/v1/query-stats?window=24h&limit=10", controller.signal)
      .then((summary) => {
        if (!controller.signal.aborted) {
          setQuerySummary(summary);
          setSummaryError("");
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setSummaryError(error instanceof Error ? error.message : "Unable to load query statistics");
      });
    return () => controller.abort();
  }, [queryLoggingEnabled, data.checked]);
  const max = Math.max(1, ...history);
  const points = history
    .map(
      (v, i) =>
        `${(i / Math.max(1, history.length - 1)) * 600},${125 - (v / max) * 100}`,
    )
    .join(" ");
  return (
    <>
      <OnboardingChecklist />
      <div className="stats">
        <Stat
          label={t("dashboard.total_queries")}
          value={number(data.stats.queries_total)}
          note={t("dashboard.since_start")}
          icon={<Activity size={17} />}
        />
        <Stat
          label={t("dashboard.qps")}
          value={data.stats.queries_per_second.toFixed(2)}
          note={t("dashboard.rolling_avg")}
          icon={<Zap size={17} />}
        />
        <Stat
          label={t("dashboard.cache_hit_rate")}
          value={`${(data.stats.cache_hit_rate * 100).toFixed(1)}%`}
          note={`${number(data.cache.hits)} ${t("dashboard.answers_from_memory")}`}
          icon={<Database size={17} />}
        />
        <Stat
          label={t("dashboard.uptime")}
          value={`${Math.floor(data.status.uptime_seconds / 3600)}h ${Math.floor(data.status.uptime_seconds / 60) % 60}m`}
          note={t("dashboard.process_lifetime")}
          icon={<Timer size={17} />}
        />
      </div>
      <div className="overview-grid">
        <RankingPanel
          title={t("dashboard.top_domains")}
          values={querySummary?.top_domains}
          enabled={queryLoggingEnabled}
          error={summaryError}
        />
        <RankingPanel
          title={t("dashboard.top_clients")}
          values={querySummary?.top_clients}
          enabled={queryLoggingEnabled}
          error={summaryError}
        />
      </div>
      <div className="overview-grid">
        <section className="panel traffic">
          <div className="panel-heading">
            <div>
              <h2>{t("dashboard.query_activity")}</h2>
              <p>{t("dashboard.live_samples")}</p>
            </div>
            <span className="subtle-badge">{t("dashboard.refresh_5s")}</span>
          </div>
          <div className="chart-label">
            <strong>{data.stats.queries_per_second.toFixed(2)}</strong>
            <span>{t("dashboard.queries_per_sec")}</span>
          </div>
          <svg
            className="chart"
            viewBox="0 0 600 145"
            role="img"
            aria-label={t("dashboard.chart_aria")}
          >
            <path d="M0 25H600 M0 75H600 M0 125H600" className="chart-grid" />
            <polyline points={points} className="chart-line" />
          </svg>
          <div className="chart-footer">
            <span>{t("dashboard.session_samples")} · {history.length} / 30</span>
            <span>{t("dashboard.now")}</span>
          </div>
        </section>
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("dashboard.resolver_pipeline")}</h2>
              <p>{t("dashboard.pipeline_subtitle")}</p>
            </div>
          </div>
          <ol className="pipeline">
            <li>
              <span>01</span>
              <div>
                <strong>{t("dashboard.client_access")}</strong>
                <p>{t("dashboard.allowed_network_check")}</p>
              </div>
              <i className="dot" />
            </li>
            {data.status.capabilities.includes("local_zones") && (
              <li>
                <span>02</span>
                <div>
                  <strong>{t("dashboard.local_zones")}</strong>
                  <p>{t("dashboard.authoritative_before_forwarding")}</p>
                </div>
                <i className="dot" />
              </li>
            )}
            <li>
              <span>03</span>
              <div>
                <strong>{t("dashboard.memory_cache")}</strong>
                <p>
                  {number(data.cache.entries)} / {number(data.cache.capacity)}{" "}
                  {t("dashboard.memory_cache")}
                </p>
              </div>
              <i className="dot" />
            </li>
            <li>
              <span>04</span>
              <div>
                <strong>{t("dashboard.upstream_forwarding")}</strong>
                <p>{data.config.dns.upstreams.length} {t("dashboard.configured_resolvers")}</p>
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
              <h2>{t("dashboard.upstream_resolvers")}</h2>
              <p>{t("dashboard.ordered_failover")}</p>
            </div>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("dashboard.endpoint")}</th>
                  <th>{t("dashboard.priority")}</th>
                  <th>{t("dashboard.attempt_timeout")}</th>
                </tr>
              </thead>
              <tbody>
                {data.config.dns.upstreams.map((upstream, i) => (
                  <tr key={upstream}>
                    <td>
                      <span className="endpoint-mark">↗</span>
                      <code>{upstream}</code>
                    </td>
                    <td>{i === 0 ? t("dashboard.primary") : `${t("dashboard.fallback")} ${i}`}</td>
                    <td>{data.config.dns.timeout / 1e9}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="panel-footnote">
            {t("dashboard.config_shown")}
          </p>
        </section>
        <section className="foundation-card">
          <span className="eyebrow">{t("dashboard.foundation_eyebrow")}</span>
          <h2>
            {t("dashboard.foundation_title_1")}
            <br />
            {t("dashboard.foundation_title_2")}
          </h2>
          <p>
            {t("dashboard.foundation_text")}
          </p>
          <a href="https://github.com/matta813/velora-dns/issues">
            {t("dashboard.explore_roadmap")} <ArrowUpRight size={16} />
          </a>
        </section>
      </div>
    </>
  );
}

function RankingPanel({ title, values, enabled, error }: { title: string; values?: { value: string; count: number }[]; enabled: boolean; error: string }) {
  const { t } = useI18n();
  return (
    <section className="panel">
      <div className="panel-heading">
        <div><h2>{title}</h2><p>{t("dashboard.retained_last_24h")}</p></div>
      </div>
      {!enabled ? (
        <p className="notice">{t("dashboard.unavailable_no_logging")}</p>
      ) : error ? (
        <p className="notice error" role="alert">{error}</p>
      ) : !values ? (
        <p role="status">{t("dashboard.loading_stats")}</p>
      ) : values.length === 0 ? (
        <p className="panel-footnote">{t("dashboard.no_retained")}</p>
      ) : (
        <ol className="ranking-list">
          {values.map((item) => <li key={item.value}><code>{item.value}</code><strong>{number(item.count)}</strong></li>)}
        </ol>
      )}
    </section>
  );
}