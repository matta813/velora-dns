import { useCallback, useEffect, useState } from "react";
import { ClipboardList, Filter } from "lucide-react";
import { request } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { EmptyState } from "../components/EmptyState";
import { toneFor } from "../components/tone";

interface AuditEvent {
  id: number;
  occurred_at: string;
  actor: string;
  role: string;
  action: string;
  target: string;
  result: string;
  status_code: number;
}

export function AuditLog() {
  const { t } = useI18n();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [result, setResult] = useState("");
  const [filters, setFilters] = useState({ actor: "", action: "", result: "" });
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [more, setMore] = useState(false);

  const load = useCallback(async (before = 0) => {
    setLoading(true);
    try {
      const query = new URLSearchParams({ limit: "50" });
      if (filters.actor) query.set("actor", filters.actor);
      if (filters.action) query.set("action", filters.action);
      if (filters.result) query.set("result", filters.result);
      if (before) query.set("before", String(before));
      const next = await request<AuditEvent[]>(`/api/v1/audit?${query}`);
      setEvents((previous) => before ? [...previous, ...next] : next);
      setMore(next.length === 50);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("audit.load_failed"));
    } finally {
      setLoading(false);
    }
  }, [filters, t]);

  useEffect(() => {
    const timer = setTimeout(() => void load(), 0);
    return () => clearTimeout(timer);
  }, [load]);

  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{t("audit.title")}</h2>
          <p>{t("audit.hint")}</p>
        </div>
      </div>
      <form
        onSubmit={(event) => { event.preventDefault(); setFilters({ actor: actor.trim(), action: action.trim(), result }); }}
        className="zone-form audit-filters"
      >
        <div className="form-grid filters">
          <label>{t("audit.actor")}<input value={actor} onChange={(event) => setActor(event.target.value)} /></label>
          <label>{t("audit.action")}<input value={action} onChange={(event) => setAction(event.target.value)} /></label>
          <label>{t("audit.result")}
            <select value={result} onChange={(event) => setResult(event.target.value)}>
              <option value="">{t("audit.all")}</option>
              <option value="success">{t("audit.success")}</option>
              <option value="failure">{t("audit.failure")}</option>
            </select>
          </label>
          <div className="filter-submit">
            <button className="button primary" type="submit"><Filter size={15} />{t("audit.filter")}</button>
          </div>
        </div>
      </form>
      {error && <div className="notice error" role="alert">{error}</div>}
      {events.length > 0 && (
        <div className="table-wrap">
          <table>
            <thead><tr><th>{t("audit.time")}</th><th>{t("audit.actor")}</th><th>{t("audit.action")}</th><th>{t("audit.target")}</th><th>{t("audit.result")}</th></tr></thead>
            <tbody>{events.map((item) => (
              <tr key={item.id}>
                <td className="cell-muted">{new Date(item.occurred_at).toLocaleString()}</td>
                <td>{item.actor} ({item.role || "—"})</td>
                <td><code>{item.action}</code></td>
                <td className="wrap"><code>{item.target}</code></td>
                <td>
                  <Badge tone={toneFor(item.result)}>
                    {`${item.result === "success" || item.result === "failure" ? t(`audit.${item.result}`) : item.result} ${item.status_code || ""}`.trim()}
                  </Badge>
                </td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      )}
      {events.length === 0 && !loading && <EmptyState compact icon={<ClipboardList size={22} />} title={t("audit.empty")} />}
      {more && (
        <div className="panel-footer">
          <span />
          <button className="button" disabled={loading} onClick={() => void load(events[events.length - 1].id)}>{t("audit.more")}</button>
        </div>
      )}
    </section>
  );
}
