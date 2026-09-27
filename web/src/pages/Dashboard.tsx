import { useEffect, useState } from "react";
import { Activity, ArrowUpRight, Database, RotateCcw, Server, Timer, Zap } from "lucide-react";
import { request, type QuerySummary, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { OnboardingChecklist } from "../components/OnboardingChecklist";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";
import { useI18n } from "../i18n-context";
import type { Language } from "../i18n";
const number = (language: Language, v: number) =>
  new Intl.NumberFormat(language).format(v);
interface UpstreamHealth {
  address: string;
  state: "unknown" | "healthy" | "degraded" | "unavailable";
  consecutive_failures: number;
  latency_milliseconds: number;
}
export function Dashboard({
  data,
  history,
  queryLoggingEnabled,
  readOnly,
  refresh,
}: {
  data: Snapshot;
  history: number[];
  queryLoggingEnabled: boolean;
  readOnly?: boolean;
  refresh?: () => void;
}) {
  const { t, language } = useI18n();
  const [querySummary, setQuerySummary] = useState<QuerySummary | null>(null);
  const [summaryError, setSummaryError] = useState("");
  const [upstreamHealth, setUpstreamHealth] = useState<UpstreamHealth[]>([]);
  const [resetError, setResetError] = useState("");
  const [resetting, setResetting] = useState(false);
  async function resetStatistics() {
    if (!window.confirm(t("dashboard.reset_confirm"))) return;
    setResetting(true);
    setResetError("");
    try {
      await request("/api/v1/stats/reset", undefined, "POST");
      refresh?.();
    } catch (reason) {
      setResetError(reason instanceof Error ? reason.message : t("dashboard.reset_failed"));
    } finally {
      setResetting(false);
    }
  }
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const health = await request<UpstreamHealth[]>("/api/v1/upstreams/health", controller.signal);
        if (!controller.signal.aborted && Array.isArray(health)) setUpstreamHealth(health);
      } catch {
        if (!controller.signal.aborted) setUpstreamHealth([]);
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
      }
    }
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);
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
          setSummaryError(error instanceof Error ? error.message : t("dashboard.stats_load_failed"));
      });
    return () => controller.abort();
  }, [queryLoggingEnabled, data.checked, t]);
  const max = Math.max(1, ...history);
  // A single sample still draws a flat line so the chart never looks broken.
  const samples = history.length === 1 ? [history[0], history[0]] : history;
  const coords = samples.map((v, i) => [
    (i / Math.max(1, samples.length - 1)) * 600,
    150 - (v / max) * 125,
  ]);
  const line = coords.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = coords.length
    ? `M0,160 L${coords.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" L")} L600,160 Z`
    : "";
  const hours = Math.floor(data.status.uptime_seconds / 3600);
  const uptime =
    hours >= 48
      ? `${Math.floor(hours / 24)}d ${hours % 24}h`
      : `${hours}h ${Math.floor(data.status.uptime_seconds / 60) % 60}m`;
  const healthFor = (address: string) => upstreamHealth.find((item) => item.address === address);
  const upstreams = upstreamHealth.length
    ? upstreamHealth.map((item) => item.address)
    : data.config.dns.upstreams;
  return (
    <div className="stack">
      <OnboardingChecklist />
      {!readOnly && refresh && (
        <div className="dashboard-actions">
          <button className="button ghost small" disabled={resetting} onClick={() => void resetStatistics()}>
            <RotateCcw size={14} />
            {t("dashboard.reset_statistics")}
          </button>
        </div>
      )}
      {resetError && <div className="notice error" role="alert">{resetError}</div>}
      <div className="stats">
        <Stat
          label={t("dashboard.total_queries")}
          value={number(language, data.stats.queries_total)}
          note={t("dashboard.since_start")}
          icon={<Activity size={17} />}
        />
        <Stat
          label={t("dashboard.qps")}
          value={data.stats.queries_per_second.toFixed(2)}
          note={t("dashboard.rolling_avg")}
          icon={<Zap size={17} />}
          tone="info"
        />
        <Stat
          label={t("dashboard.cache_hit_rate")}
          value={`${(data.stats.cache_hit_rate * 100).toFixed(1)}%`}
          note={`${number(language, data.cache.hits)} ${t("dashboard.answers_from_memory")}`}
          icon={<Database size={17} />}
        />
        <Stat
          label={t("dashboard.uptime")}
          value={uptime}
          note={t("dashboard.process_lifetime")}
          icon={<Timer size={17} />}
          tone="warning"
        />
      </div>
      <div className="grid-main-side">
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
          <div className="chart-wrap">
            <svg
              className="chart"
              viewBox="0 0 600 160"
              preserveAspectRatio="none"
              role="img"
              aria-label={t("dashboard.chart_aria")}
            >
              <defs>
                <linearGradient id="chart-gradient" x1="0" x2="0" y1="0" y2="1">
                  <stop offset="0%" stopColor="var(--chart-line)" stopOpacity="0.28" />
                  <stop offset="100%" stopColor="var(--chart-line)" stopOpacity="0" />
                </linearGradient>
              </defs>
              <path d="M0 25H600 M0 87.5H600 M0 150H600" className="chart-grid" />
              {area && <path d={area} className="chart-area" />}
              {line && <polyline points={line} className="chart-line" />}
            </svg>
            {history.length < 2 && <div className="chart-empty">{t("dashboard.collecting_samples")}</div>}
          </div>
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
              <span className="step-index">01</span>
              <div>
                <strong>{t("dashboard.client_access")}</strong>
                <p>{t("dashboard.allowed_network_check")}</p>
              </div>
              <i className="dot" />
            </li>
            {data.status.capabilities.includes("local_zones") && (
              <li>
                <span className="step-index">02</span>
                <div>
                  <strong>{t("dashboard.local_zones")}</strong>
                  <p>{t("dashboard.authoritative_before_forwarding")}</p>
                </div>
                <i className="dot" />
              </li>
            )}
            <li>
              <span className="step-index">03</span>
              <div>
                <strong>{t("dashboard.memory_cache")}</strong>
                <p>
                  {number(language, data.cache.entries)} / {number(language, data.cache.capacity)}
                </p>
              </div>
              <i className="dot" />
            </li>
            <li>
              <span className="step-index">04</span>
              <div>
                <strong>{t("dashboard.upstream_forwarding")}</strong>
                <p>{data.config.dns.upstreams.length} {t("dashboard.configured_resolvers")}</p>
              </div>
              <ArrowUpRight size={16} />
            </li>
          </ol>
        </section>
      </div>
      <div className="grid-2">
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
      <div className="grid-main-side">
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
                  <th>{t("dashboard.health")}</th>
                </tr>
              </thead>
              <tbody>
                {upstreams.map((upstream, i) => {
                  const health = healthFor(upstream);
                  const state = health?.state ?? "unknown";
                  return (
                    <tr key={upstream}>
                      <td>
                        <span className="endpoint">
                          <Server size={14} />
                          <code>{upstream}</code>
                        </span>
                      </td>
                      <td>
                        {i === 0 ? (
                          <Badge tone="brand" plain>{t("dashboard.primary")}</Badge>
                        ) : (
                          <span className="cell-muted">{`${t("dashboard.fallback")} ${i}`}</span>
                        )}
                      </td>
                      <td className="cell-muted">{data.config.dns.timeout / 1e9}s</td>
                      <td>
                        <Badge tone={toneFor(state)}>{t(`dashboard.upstream_${state}`)}</Badge>
                        {state === "healthy" && health && (
                          <span className="latency">{Math.round(health.latency_milliseconds)} ms</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <p className="panel-footnote">{t("dashboard.config_shown")}</p>
        </section>
        <section className="foundation-card">
          <span className="eyebrow">{t("dashboard.foundation_eyebrow")}</span>
          <h2>
            {t("dashboard.foundation_title_1")}
            <br />
            {t("dashboard.foundation_title_2")}
          </h2>
          <p>{t("dashboard.foundation_text")}</p>
          <a href="https://github.com/matta813/velora-dns/issues">
            {t("dashboard.explore_roadmap")} <ArrowUpRight size={15} />
          </a>
        </section>
      </div>
    </div>
  );
}

function RankingPanel({ title, values, enabled, error }: { title: string; values?: { value: string; count: number }[]; enabled: boolean; error: string }) {
  const { t, language } = useI18n();
  const top = Math.max(1, ...(values ?? []).map((item) => item.count));
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
        <Loading>{t("dashboard.loading_stats")}</Loading>
      ) : values.length === 0 ? (
        <EmptyState compact title={t("dashboard.no_retained")} />
      ) : (
        <ol className="ranking-list">
          {values.map((item) => (
            <li key={item.value}>
              <code title={item.value}>{item.value}</code>
              <strong>{number(language, item.count)}</strong>
              <span className="meter" aria-hidden="true"><span style={{ width: `${(item.count / top) * 100}%` }} /></span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
