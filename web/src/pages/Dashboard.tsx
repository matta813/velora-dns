import { useEffect, useState } from "react";
import { RotateCcw } from "lucide-react";
import { request, type QuerySummary, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { OnboardingChecklist } from "../components/OnboardingChecklist";
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
  const peak = Math.max(...history, 0);
  const max = niceCeiling(peak);
  // A single sample still draws a flat line so the chart never looks broken.
  const samples = history.length === 1 ? [history[0], history[0]] : history;
  const coords = samples.map((v, i) => [
    (i / Math.max(1, samples.length - 1)) * 600,
    200 - (v / max) * 200,
  ]);
  const line = coords.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = coords.length
    ? `M0,200 L${coords.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" L")} L600,200 Z`
    : "";
  const hours = Math.floor(data.status.uptime_seconds / 3600);
  const uptime =
    hours >= 48
      ? `${Math.floor(hours / 24)}d ${hours % 24}h`
      : `${hours}h ${Math.floor(data.status.uptime_seconds / 60) % 60}m`;
  const total = data.stats.queries_total;
  const blocked = data.stats.blocked_queries ?? 0;
  const blockedShare = total > 0 ? blocked / total : 0;
  const healthFor = (address: string) => upstreamHealth.find((item) => item.address === address);
  const upstreams = upstreamHealth.length
    ? upstreamHealth.map((item) => item.address)
    : data.config.dns.upstreams;
  const percent = (value: number) => `${(value * 100).toFixed(1)}%`;
  return (
    <div className="stack">
      <OnboardingChecklist />
      {resetError && <div className="notice error" role="alert">{resetError}</div>}
      <div className="stats">
        <Stat
          label={t("dashboard.total_queries")}
          value={number(language, total)}
          note={t("dashboard.since_start")}
          tone="info"
        />
        <Stat
          label={t("dashboard.blocked")}
          value={number(language, blocked)}
          aside={percent(blockedShare)}
          note={`${percent(blockedShare)} ${t("dashboard.of_all_queries")}`}
          progress={blockedShare}
          tone="danger"
        />
        <Stat
          label={t("dashboard.cache_hit_rate")}
          value={percent(data.stats.cache_hit_rate)}
          note={`${number(language, data.cache.hits)} ${t("dashboard.answers_from_memory")}`}
          progress={data.stats.cache_hit_rate}
          tone="success"
        />
        <Stat
          label={t("dashboard.qps")}
          value={data.stats.queries_per_second.toFixed(2)}
          note={t("dashboard.rolling_avg")}
          tone="warning"
        />
      </div>
      <div className="grid-main-side">
        <section className="panel">
          <div className="panel-heading">
            <h2>{t("dashboard.query_activity")}</h2>
            <span className="subtle-badge">{t("dashboard.refresh_5s")}</span>
          </div>
          <div className="chart-wrap">
            <div className="chart-axis" aria-hidden="true">
              <span>{formatAxis(max)}</span>
              <span>{formatAxis(max / 2)}</span>
              <span>0</span>
            </div>
            <svg
              className="chart"
              viewBox="0 0 600 200"
              preserveAspectRatio="none"
              role="img"
              aria-label={t("dashboard.chart_aria")}
            >
              <path d="M0 0.5H600 M0 100H600 M0 199.5H600" className="chart-grid" />
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
            <h2>{t("dashboard.general_statistics")}</h2>
            {!readOnly && refresh && (
              <button className="button small" disabled={resetting} onClick={() => void resetStatistics()}>
                <RotateCcw size={14} />
                {t("dashboard.reset_statistics")}
              </button>
            )}
          </div>
          <table className="kv-table">
            <tbody>
              <tr>
                <td>{t("dashboard.uptime")}</td>
                <td>{uptime}</td>
              </tr>
              <tr>
                <td>{t("cache.live_entries")}</td>
                <td>
                  {number(language, data.cache.entries)} / {number(language, data.cache.capacity)}
                </td>
              </tr>
              <tr>
                <td>{t("cache.hits")}</td>
                <td>{number(language, data.cache.hits)}</td>
              </tr>
              <tr>
                <td>{t("cache.misses")}</td>
                <td>{number(language, data.cache.misses)}</td>
              </tr>
              <tr>
                <td>{t("dashboard.upstream_servers")}</td>
                <td>{data.config.dns.upstreams.length}</td>
              </tr>
              <tr>
                <td>{t("dashboard.query_logging")}</td>
                <td>{queryLoggingEnabled ? t("dashboard.enabled") : t("dashboard.disabled")}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>
      <div className="grid-2">
        <RankingPanel
          title={t("dashboard.top_clients")}
          column={t("dashboard.client")}
          values={querySummary?.top_clients}
          total={querySummary?.total}
          enabled={queryLoggingEnabled}
          error={summaryError}
        />
        <RankingPanel
          title={t("dashboard.top_domains")}
          column={t("dashboard.domain")}
          values={querySummary?.top_domains}
          total={querySummary?.total}
          enabled={queryLoggingEnabled}
          error={summaryError}
        />
      </div>
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
                    <td><code>{upstream}</code></td>
                    <td>{i === 0 ? t("dashboard.primary") : `${t("dashboard.fallback")} ${i}`}</td>
                    <td>{data.config.dns.timeout / 1e9}s</td>
                    <td>
                      <span className={`status-text ${toneFor(state)}`}>{t(`dashboard.upstream_${state}`)}</span>
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
    </div>
  );
}

/** Rounds a chart maximum up to 1, 2 or 5 × 10^n so axis labels stay readable. */
function niceCeiling(value: number) {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const step = [1, 2, 5, 10].find((m) => m * magnitude >= value) ?? 10;
  return step * magnitude;
}

function formatAxis(value: number) {
  return value >= 10 ? String(Math.round(value)) : value.toFixed(1).replace(/\.0$/, "");
}

function RankingPanel({
  title,
  column,
  values,
  total,
  enabled,
  error,
}: {
  title: string;
  column: string;
  values?: { value: string; count: number }[];
  total?: number;
  enabled: boolean;
  error: string;
}) {
  const { t, language } = useI18n();
  const denominator = Math.max(1, total ?? 0, ...(values ?? []).map((item) => item.count));
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{title}</h2>
          <p>{t("dashboard.retained_last_24h")}</p>
        </div>
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
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{column}</th>
                <th className="num">{t("dashboard.requests")}</th>
              </tr>
            </thead>
            <tbody>
              {values.map((item) => {
                const share = item.count / denominator;
                return (
                  <tr key={item.value}>
                    <td className="wrap"><code>{item.value}</code></td>
                    <td>
                      <div className="share">
                        <strong>{number(language, item.count)}</strong>
                        <span className="share-bar" aria-hidden="true">
                          <span style={{ width: `${share * 100}%` }} />
                        </span>
                        <span className="share-pct">{(share * 100).toFixed(1)}%</span>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
