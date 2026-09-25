import { useEffect, useState } from "react";
import { Download } from "lucide-react";
import { request } from "../api";
import { useI18n } from "../i18n-context";

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
  if (!report) return <section className="panel padded" role="status">{t("diagnostics.loading")}</section>;
  return (
    <section className="panel padded">
      <div className="panel-heading">
        <div>
          <h2>{t("diagnostics.title")}</h2>
          <p>{t("diagnostics.report_hint")}</p>
        </div>
        <button className="button" onClick={download}><Download size={15} />{t("diagnostics.download")}</button>
      </div>
      <p role="status"><strong>{t(`diagnostics.state.${report.state}`)}</strong> · Velora DNS {report.version.version}</p>
      <p>{t("diagnostics.upstreams")}: {report.upstream_count} · {report.os}/{report.architecture}</p>
      <div className="table-wrap">
        <table>
          <thead><tr><th>{t("diagnostics.component")}</th><th>{t("diagnostics.state")}</th><th>{t("diagnostics.detail")}</th></tr></thead>
          <tbody>{report.components.map((component) => (
            <tr key={component.name}>
              <td>{component.name.replaceAll("_", " ")}</td>
              <td>{t(`diagnostics.state.${component.state}`)}</td>
              <td>{component.detail}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </section>
  );
}
