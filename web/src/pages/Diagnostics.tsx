import { useEffect, useState } from "react";
import { Activity, Download } from "lucide-react";
import { request } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";
import { AnswerExplainer } from "./AnswerExplainer";

interface DiagnosticComponent {
  name: string;
  state: "healthy" | "degraded" | "failed";
  detail: string;
}
interface DiagnosticReport {
  generated_at: string;
  version: { version: string; commit: string; built: string };
  os: string;
  architecture: string;
  uptime_seconds: number;
  state: "healthy" | "degraded" | "failed";
  components: DiagnosticComponent[];
  upstream_count: number;
  query_log_enabled: boolean;
}

export function Diagnostics() {
  const { t } = useI18n();
  const [report, setReport] = useState<DiagnosticReport | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void request<DiagnosticReport>("/api/v1/diagnostics", controller.signal)
      .then((value) => { if (!controller.signal.aborted) setReport(value); })
      .catch((reason) => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : t("diagnostics.load_failed")); });
    return () => controller.abort();
  }, [t]);

  function download() {
    if (!report) return;
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "velora-diagnostics.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  }

  if (error) return <div className="notice error" role="alert">{error}</div>;
  if (!report) return <section className="panel"><Loading>{t("diagnostics.loading")}</Loading></section>;
  const hours = Math.floor(report.uptime_seconds / 3600);
  return (
    <div className="stack">
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>{t("diagnostics.title")}</h2>
            <p>{t("diagnostics.report_hint")}</p>
          </div>
          <button className="button" onClick={download}><Download size={15} />{t("diagnostics.download")}</button>
        </div>
        <div className="panel-body">
          <div className="version-hero">
            <span className={`version-icon ${report.state === "healthy" ? "success" : report.state === "failed" ? "danger" : ""}`} aria-hidden="true">
              <Activity size={22} />
            </span>
            <div>
              <p role="status" className="diagnostic-summary">
                <Badge tone={toneFor(report.state)}>{t(`diagnostics.state.${report.state}`)}</Badge>
                <strong>Velora DNS {report.version.version}</strong>
              </p>
              <span>
                {t("diagnostics.upstreams")}: {report.upstream_count} · {report.os}/{report.architecture} · {hours}h {Math.floor(report.uptime_seconds / 60) % 60}m
              </span>
            </div>
          </div>
        </div>
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("diagnostics.component")}</th><th>{t("diagnostics.state")}</th><th>{t("diagnostics.detail")}</th></tr></thead>
            <tbody>{report.components.map((component) => (
              <tr key={component.name}>
                <td><strong className="capitalize">{component.name.replaceAll("_", " ")}</strong></td>
                <td><Badge tone={toneFor(component.state)}>{t(`diagnostics.state.${component.state}`)}</Badge></td>
                <td className="wrap cell-muted">{component.detail}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </section>
      <AnswerExplainer />
    </div>
  );
}
