import { useCallback, useEffect, useMemo, useState, type KeyboardEvent, type MouseEvent } from "react";
import { Link } from "react-router-dom";
import { BarChart3, RefreshCw } from "lucide-react";
import { request, type QueryRanking, type QuerySummary } from "../api";
import { loadAnalytics, type Analytics as AnalyticsData, type AnalyticsBucket, type AnalyticsRange } from "../api-analytics";
import { EmptyState, Loading } from "../components/EmptyState";
import { Stat } from "../components/Stat";
import { useI18n } from "../i18n-context";
import "./analytics.css";

const RANGES: AnalyticsRange[] = ["1h", "24h", "7d", "30d"];

const percent = (part: number, whole: number) => (whole > 0 ? (part / whole) * 100 : 0);

/** Rounds a maximum up to 1, 2 or 5 × 10^n so axis labels stay readable. */
function niceMax(value: number) {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const step = [1, 2, 5, 10].find((f) => f * magnitude >= value) ?? 10;
  return step * magnitude;
}

function bucketLabel(start: string, seconds: number, range: AnalyticsRange, language: string) {
  const date = new Date(start);
  if (seconds >= 86400) return date.toLocaleDateString(language, { month: "short", day: "numeric" });
  if (range === "7d") return date.toLocaleString(language, { weekday: "short", hour: "2-digit", minute: "2-digit" });
  return date.toLocaleTimeString(language, { hour: "2-digit", minute: "2-digit" });
}

export function Analytics({ queryLogging }: { queryLogging: boolean }) {
  const { t, language } = useI18n();
  const [range, setRange] = useState<AnalyticsRange>("24h");
  const [data, setData] = useState<AnalyticsData | null>(null);
  const [summary, setSummary] = useState<QuerySummary | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      const [next, top] = await Promise.all([
        loadAnalytics(range, signal),
        request<QuerySummary>(`/api/v1/query-stats?window=${range}&limit=10`, signal),
      ]);
      if (!signal?.aborted) {
        setData(next);
        setSummary(top);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("analytics.load_failed"));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [range, t]);

  useEffect(() => {
    if (!queryLogging) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [load, queryLogging]);

  const number = (value: number) => value.toLocaleString(language);

  if (!queryLogging) {
    return (
      <section className="panel">
        <EmptyState icon={<BarChart3 size={22} />} title={t("analytics.logging_off_title")}>
          {t("analytics.logging_off_text")} <Link to="/settings">{t("app.nav.settings")}</Link>
        </EmptyState>
      </section>
    );
  }

  const totals = data?.totals;
  const limitedHistory = data?.history_start && new Date(data.history_start) > new Date(data.window_start);
  return (
    <div className="stack">
      <div className="zones-toolbar analytics-toolbar">
        <div className="segmented" role="group" aria-label={t("analytics.range")}>
          {RANGES.map((value) => (
            <button key={value} type="button" aria-pressed={range === value} onClick={() => setRange(value)}>
              {t(`analytics.range_${value}`)}
            </button>
          ))}
        </div>
        <button className="button" disabled={loading} onClick={() => void load()}>
          <RefreshCw size={15} />
          {t("blocklists.reload")}
        </button>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {limitedHistory && data?.history_start && (
        <div className="notice info" role="status">
          {t("analytics.history_limited")} {new Date(data.history_start).toLocaleString(language)}.
        </div>
      )}
      {!data && !error && (
        <section className="panel">
          <Loading>{t("app.connecting_panel")}</Loading>
        </section>
      )}
      {data && totals && (
        <>
          <div className="stats">
            <Stat label={t("analytics.queries")} value={number(totals.total)} note={t(`analytics.range_${range}`)} tone="info" />
            <Stat label={t("analytics.blocked")} value={number(totals.blocked)} aside={`${percent(totals.blocked, totals.total).toFixed(1)}%`} progress={percent(totals.blocked, totals.total) / 100} tone="danger" />
            <Stat label={t("analytics.cached")} value={`${percent(totals.cached, totals.total).toFixed(1)}%`} note={`${number(totals.cached)} ${t("analytics.answers")}`} tone="success" />
            <Stat label={t("analytics.avg_upstream")} value={totals.average_ms ? `${totals.average_ms.toFixed(1)} ms` : "—"} note={`${number(totals.failed)} SERVFAIL`} tone={totals.failed > 0 ? "warning" : "neutral"} />
          </div>
          {totals.total === 0 ? (
            <section className="panel">
              <EmptyState icon={<BarChart3 size={22} />} title={t("analytics.empty_title")}>{t("analytics.empty_text")}</EmptyState>
            </section>
          ) : (
            <>
              <QueryChart data={data} range={range} />
              <LatencyChart data={data} range={range} />
              <div className="grid-2">
                <Breakdown title={t("analytics.query_types")} values={data.query_types} total={totals.total} />
                <Breakdown title={t("analytics.sources")} values={data.sources.map((item) => ({ ...item, value: t(`analytics.source_${item.value}`) }))} total={totals.total} plain />
                <Breakdown title={t("analytics.response_codes")} values={data.response_codes} total={totals.total} />
                <Breakdown title={t("analytics.top_blocked")} values={data.top_blocked} total={totals.blocked} empty={t("analytics.nothing_blocked")} />
                {summary && <Breakdown title={t("dashboard.top_domains")} values={summary.top_domains} total={summary.total} />}
                {summary && <Breakdown title={t("dashboard.top_clients")} values={summary.top_clients} total={summary.total} />}
              </div>
              <section className="panel">
                <div className="panel-heading">
                  <div>
                    <h2>{t("analytics.upstreams")}</h2>
                    <p>{t("analytics.upstreams_hint")}</p>
                  </div>
                </div>
                {data.upstreams.length === 0 ? (
                  <EmptyState compact title={t("analytics.no_upstream")} />
                ) : (
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>{t("dashboard.endpoint")}</th>
                          <th className="num">{t("analytics.queries")}</th>
                          <th className="num">{t("analytics.failures")}</th>
                          <th className="num">{t("analytics.avg_time")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {data.upstreams.map((upstream) => (
                          <tr key={upstream.address}>
                            <td><code>{upstream.address}</code></td>
                            <td className="num mono">{number(upstream.queries)}</td>
                            <td className={`num mono${upstream.failed ? " text-danger" : ""}`}>
                              {number(upstream.failed)} <small>({percent(upstream.failed, upstream.queries).toFixed(1)}%)</small>
                            </td>
                            <td className="num mono">{upstream.average_ms ? `${upstream.average_ms.toFixed(1)} ms` : "—"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </section>
            </>
          )}
        </>
      )}
    </div>
  );
}

/** Shared hover/keyboard selection for the bucketed charts. */
function useActiveBucket(length: number) {
  const [active, setActive] = useState<number | null>(null);
  const onMove = (event: MouseEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width === 0) return;
    setActive(Math.max(0, Math.min(length - 1, Math.floor(((event.clientX - rect.left) / rect.width) * length))));
  };
  const onKey = (event: KeyboardEvent<SVGSVGElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    setActive((current) => {
      const from = current ?? length - 1;
      return Math.max(0, Math.min(length - 1, from + (event.key === "ArrowLeft" ? -1 : 1)));
    });
  };
  return { active, setActive, onMove, onKey };
}

function QueryChart({ data, range }: { data: AnalyticsData; range: AnalyticsRange }) {
  const { t, language } = useI18n();
  const series = data.series;
  const max = niceMax(Math.max(...series.map((bucket) => bucket.total)));
  const { active, setActive, onMove, onKey } = useActiveBucket(series.length);
  const shown: AnalyticsBucket = series[active ?? series.length - 1];
  const width = series.length * 10;
  const label = (bucket: AnalyticsBucket) => bucketLabel(bucket.start, data.bucket_seconds, range, language);
  const ticks = [0, Math.floor(series.length / 2), series.length - 1];
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{t("analytics.over_time")}</h2>
          <p>{t("analytics.over_time_hint")}</p>
        </div>
        <div className="chart-legend" aria-hidden="true">
          <span><i className="swatch allowed" /> {t("analytics.allowed")}</span>
          <span><i className="swatch blocked" /> {t("analytics.blocked")}</span>
        </div>
      </div>
      <div className="chart-wrap">
        <div className="chart-axis" aria-hidden="true">
          <span>{max.toLocaleString(language)}</span>
          <span>{(max / 2).toLocaleString(language)}</span>
          <span>0</span>
        </div>
        <svg
          className="chart bars"
          viewBox={`0 0 ${width} 200`}
          preserveAspectRatio="none"
          role="img"
          tabIndex={0}
          aria-label={t("analytics.chart_aria")}
          onMouseMove={onMove}
          onMouseLeave={() => setActive(null)}
          onKeyDown={onKey}
        >
          <path d={`M0 0.5H${width} M0 100H${width} M0 199.5H${width}`} className="chart-grid" />
          {series.map((bucket, i) => {
            const allowed = ((bucket.total - bucket.blocked) / max) * 200;
            const blocked = (bucket.blocked / max) * 200;
            return (
              <g key={bucket.start} className={active === i ? "active" : undefined}>
                <rect className="bar-hit" x={i * 10} y={0} width={10} height={200} />
                <rect className="bar-allowed" x={i * 10 + 1} y={200 - allowed - blocked} width={8} height={allowed} />
                <rect className="bar-blocked" x={i * 10 + 1} y={200 - blocked} width={8} height={blocked} />
              </g>
            );
          })}
        </svg>
      </div>
      <div className="chart-ticks" aria-hidden="true">
        {ticks.map((i) => <span key={i}>{label(series[i])}</span>)}
      </div>
      <p className="chart-detail" aria-live="polite">
        <strong>{label(shown)}</strong>
        <span>{t("analytics.queries")}: {shown.total.toLocaleString(language)}</span>
        <span>{t("analytics.blocked")}: {shown.blocked.toLocaleString(language)}</span>
        <span>{t("analytics.cached")}: {shown.cached.toLocaleString(language)}</span>
        <span>SERVFAIL: {shown.failed.toLocaleString(language)}</span>
      </p>
      <details className="chart-table">
        <summary>{t("analytics.show_table")}</summary>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("analytics.time")}</th>
                <th className="num">{t("analytics.queries")}</th>
                <th className="num">{t("analytics.blocked")}</th>
                <th className="num">{t("analytics.cached")}</th>
                <th className="num">SERVFAIL</th>
                <th className="num">{t("analytics.avg_time")}</th>
              </tr>
            </thead>
            <tbody>
              {series.filter((bucket) => bucket.total > 0).map((bucket) => (
                <tr key={bucket.start}>
                  <td>{label(bucket)}</td>
                  <td className="num mono">{bucket.total.toLocaleString(language)}</td>
                  <td className="num mono">{bucket.blocked.toLocaleString(language)}</td>
                  <td className="num mono">{bucket.cached.toLocaleString(language)}</td>
                  <td className="num mono">{bucket.failed.toLocaleString(language)}</td>
                  <td className="num mono">{bucket.average_ms ? bucket.average_ms.toFixed(1) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </section>
  );
}

function LatencyChart({ data, range }: { data: AnalyticsData; range: AnalyticsRange }) {
  const { t, language } = useI18n();
  const series = data.series;
  const max = niceMax(Math.max(...series.map((bucket) => bucket.average_ms)));
  const { active, setActive, onMove, onKey } = useActiveBucket(series.length);
  const width = series.length * 10;
  // Buckets without upstream traffic leave gaps instead of dropping to zero.
  const segments = useMemo(() => {
    const out: string[][] = [];
    let current: string[] = [];
    series.forEach((bucket, i) => {
      if (bucket.average_ms > 0) {
        current.push(`${i * 10 + 5},${(200 - (bucket.average_ms / max) * 200).toFixed(1)}`);
      } else if (current.length) {
        out.push(current);
        current = [];
      }
    });
    if (current.length) out.push(current);
    return out;
  }, [series, max]);
  const shown = series[active ?? series.length - 1];
  const ticks = [0, Math.floor(series.length / 2), series.length - 1];
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{t("analytics.latency")}</h2>
          <p>{t("analytics.latency_hint")}</p>
        </div>
      </div>
      <div className="chart-wrap">
        <div className="chart-axis" aria-hidden="true">
          <span>{max.toLocaleString(language)} ms</span>
          <span>{(max / 2).toLocaleString(language)}</span>
          <span>0</span>
        </div>
        <svg className="chart" viewBox={`0 0 ${width} 200`} preserveAspectRatio="none" role="img" tabIndex={0} aria-label={t("analytics.latency_aria")} onMouseMove={onMove} onMouseLeave={() => setActive(null)} onKeyDown={onKey}>
          <path d={`M0 0.5H${width} M0 100H${width} M0 199.5H${width}`} className="chart-grid" />
          {segments.map((points) => (
            points.length === 1
              ? <circle key={points[0]} className="chart-dot" cx={points[0].split(",")[0]} cy={points[0].split(",")[1]} r={3} />
              : <polyline key={points[0]} points={points.join(" ")} className="chart-line" />
          ))}
          {active !== null && <line className="chart-cursor" x1={active * 10 + 5} x2={active * 10 + 5} y1={0} y2={200} />}
        </svg>
      </div>
      <div className="chart-ticks" aria-hidden="true">
        {ticks.map((i) => <span key={i}>{bucketLabel(series[i].start, data.bucket_seconds, range, language)}</span>)}
      </div>
      <p className="chart-detail" aria-live="polite">
        <strong>{bucketLabel(shown.start, data.bucket_seconds, range, language)}</strong>
        <span>{t("analytics.avg_time")}: {shown.average_ms ? `${shown.average_ms.toFixed(1)} ms` : "—"}</span>
      </p>
    </section>
  );
}

function Breakdown({ title, values, total, empty, plain = false }: { title: string; values: QueryRanking[]; total: number; empty?: string; plain?: boolean }) {
  const { t, language } = useI18n();
  const denominator = Math.max(1, total);
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{title}</h2>
        </div>
      </div>
      {values.length === 0 ? (
        <EmptyState compact title={empty ?? t("dashboard.no_retained")} />
      ) : (
        <div className="table-wrap">
          <table>
            <tbody>
              {values.map((item) => {
                const share = item.count / denominator;
                return (
                  <tr key={item.value}>
                    <td className="wrap">
                      {item.name ? <>{item.name} <small><code>{item.value}</code></small></> : plain ? item.value : <code>{item.value}</code>}
                    </td>
                    <td>
                      <div className="share">
                        <strong>{item.count.toLocaleString(language)}</strong>
                        <span className="share-bar" aria-hidden="true">
                          <span style={{ width: `${Math.min(1, share) * 100}%` }} />
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
