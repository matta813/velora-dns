import { useState } from "react";
import { Database, Eraser, Search, Zap } from "lucide-react";
import { request, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
import { useI18n } from "../i18n-context";
export function CachePage({
  data,
  refresh,
  readOnly = false,
}: {
  data: Snapshot;
  refresh: () => void;
  readOnly?: boolean;
}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  async function flush() {
    setBusy(true);
    setMessage("");
    try {
      await request("/api/v1/cache", undefined, "DELETE");
      setMessage(t("cache.cleared"));
      refresh();
    } catch (e) {
      setMessage(e instanceof Error ? e.message : t("cache.flush_failed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="stats">
        <Stat
          label={t("cache.live_entries")}
          value={String(data.cache.entries)}
          note={`${t("cache.capacity")}: ${data.cache.capacity}`}
          icon={<Database size={17} />}
        />
        <Stat
          label={t("cache.hits")}
          value={String(data.cache.hits)}
          note={t("cache.since_startup")}
          icon={<Zap size={17} />}
        />
        <Stat
          label={t("cache.misses")}
          value={String(data.cache.misses)}
          note={t("cache.includes_uncacheable")}
          icon={<Search size={17} />}
        />
      </div>
      <section className="panel padded">
        <h2>{t("cache.memory_cache")}</h2>
        <p>
          {t("cache.description")}
        </p>
        <p>
          {t("cache.clear_description")}
        </p>
        <button
          className="button danger"
          disabled={busy || readOnly}
          onClick={() => void flush()}
        >
          <Eraser size={16} />
          {busy ? t("cache.clearing") : t("cache.clear")}
        </button>
        {message && <p role="status">{message}</p>}
      </section>
    </>
  );
}
