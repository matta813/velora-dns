import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlertOctagon, AlertTriangle, BellOff, Check, Info } from "lucide-react";
import { request, type SystemEventPage } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";

export function EventCenter({ onRead }: { onRead: () => void }) {
  const { t, language } = useI18n();
  const [page, setPage] = useState<SystemEventPage | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const events = await request<SystemEventPage>("/api/v1/events?limit=100");
      setPage(events);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("events.load_failed"));
    }
  }, [t]);
  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    const timer = window.setInterval(() => void load(), 10000);
    return () => { window.clearTimeout(initial); window.clearInterval(timer); };
  }, [load]);
  const markRead = async (id: number) => {
    try {
      await request(`/api/v1/events/${id}/read`, undefined, "POST");
      await load();
      onRead();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("events.load_failed"));
    }
  };
  const icons = { info: <Info size={17} />, warning: <AlertTriangle size={17} />, critical: <AlertOctagon size={17} /> };
  return <section className="panel">
    {error && <div className="notice error" role="alert">{error}</div>}
    {!page && !error && <Loading>{t("app.connecting")}</Loading>}
    {page && page.events.length === 0 && <EmptyState icon={<BellOff size={22} />} title={t("events.empty")} />}
    {page && page.events.length > 0 && <div className="event-list">
      {page.events.map((event) => <article className={`system-event event-${event.severity}${event.read ? " is-read" : ""}`} key={event.id}>
        <span className="event-icon" aria-hidden="true">{icons[event.severity as keyof typeof icons] ?? icons.info}</span>
        <div className="system-event-heading">
          <strong>{event.title}</strong>
          <Badge tone={event.severity === "critical" ? "danger" : event.severity === "warning" ? "warning" : "info"} plain>
            <span className="event-severity">{t(`events.${event.severity}`)}</span>
          </Badge>
          <time dateTime={event.occurred_at}>{new Date(event.occurred_at).toLocaleString(language)}</time>
        </div>
        <p>{event.message}</p>
        <div className="system-event-actions">
          {event.repeat_count > 1 && <span>{t("events.repeated")}: {event.repeat_count}</span>}
          {event.link && <Link to={event.link}>{t("events.open")}</Link>}
          {!event.read && <button className="button small" onClick={() => void markRead(event.id)}><Check size={14} />{t("events.mark_read")}</button>}
        </div>
      </article>)}
    </div>}
  </section>;
}
