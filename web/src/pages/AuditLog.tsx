import { useCallback, useEffect, useState } from "react";
import { request } from "../api";
import { useI18n } from "../i18n-context";

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
    <section className="panel padded">
      <h2>{t("audit.title")}</h2>
      <p>{t("audit.hint")}</p>
      <form onSubmit={(event) => { event.preventDefault(); setFilters({ actor: actor.trim(), action: action.trim(), result }); }} className="settings-preferences">
        <label>{t("audit.actor")}<input value={actor} onChange={(event) => setActor(event.target.value)} /></label>
        <label>{t("audit.action")}<input value={action} onChange={(event) => setAction(event.target.value)} /></label>
        <label>{t("audit.result")}
          <select value={result} onChange={(event) => setResult(event.target.value)}>
            <option value="">{t("audit.all")}</option>
            <option value="success">{t("audit.success")}</option>
            <option value="failure">{t("audit.failure")}</option>
          </select>
        </label>
        <button className="button" type="submit">{t("audit.filter")}</button>
      </form>
      {error && <div className="notice error" role="alert">{error}</div>}
      <div className="table-wrap">
        <table>
          <thead><tr><th>{t("audit.time")}</th><th>{t("audit.actor")}</th><th>{t("audit.action")}</th><th>{t("audit.target")}</th><th>{t("audit.result")}</th></tr></thead>
          <tbody>{events.map((item) => (
            <tr key={item.id}>
              <td>{new Date(item.occurred_at).toLocaleString()}</td>
              <td>{item.actor} ({item.role || "—"})</td>
              <td>{item.action}</td>
              <td><code>{item.target}</code></td>
              <td>{item.result === "success" || item.result === "failure" ? t(`audit.${item.result}`) : item.result} {item.status_code || ""}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
      {events.length === 0 && !loading && <p>{t("audit.empty")}</p>}
      {more && <button className="button" disabled={loading} onClick={() => void load(events[events.length - 1].id)}>{t("audit.more")}</button>}
    </section>
  );
}
