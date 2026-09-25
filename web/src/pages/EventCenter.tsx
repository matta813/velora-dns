import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { request, type SystemEventPage } from "../api";
import { useI18n } from "../i18n-context";

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
  return <section className="panel padded">
    {error && <div className="notice error" role="alert">{error}</div>}
    {page && page.events.length === 0 && <p>{t("events.empty")}</p>}
    {page?.events.map((event) => <article className={`system-event event-${event.severity}${event.read ? " is-read" : ""}`} key={event.id}>
      <div className="system-event-heading">
        <strong>{event.title}</strong>
        <span className="event-severity">{t(`events.${event.severity}`)}</span>
        <time dateTime={event.occurred_at}>{new Date(event.occurred_at).toLocaleString(language)}</time>
      </div>
      <p>{event.message}</p>
      <div className="system-event-actions">
        {event.repeat_count > 1 && <span>{t("events.repeated")}: {event.repeat_count}</span>}
        {event.link && <Link to={event.link}>{t("events.open")}</Link>}
        {!event.read && <button className="button secondary" onClick={() => void markRead(event.id)}>{t("events.mark_read")}</button>}
      </div>
    </article>)}
  </section>;
}
