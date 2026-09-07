import { useState } from "react";
import { Database, Eraser, Search, Zap } from "lucide-react";
import { request, type Snapshot } from "../api";
import { Stat } from "../components/Stat";
export function CachePage({
  data,
  refresh,
}: {
  data: Snapshot;
  refresh: () => void;
}) {
  const [busy, setBusy] = useState(false),
    [message, setMessage] = useState("");
  async function flush() {
    setBusy(true);
    setMessage("");
    try {
      await request("/api/v1/cache", undefined, "DELETE");
      setMessage(
        "Cache cleared. Local zones remain active; other queries will use upstream resolvers.",
      );
      refresh();
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "Cache flush failed");
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="stats">
        <Stat
          label="Live entries"
          value={String(data.cache.entries)}
          note={`Capacity: ${data.cache.capacity}`}
          icon={<Database size={17} />}
        />
        <Stat
          label="Cache hits"
          value={String(data.cache.hits)}
          note="Since process startup"
          icon={<Zap size={17} />}
        />
        <Stat
          label="Cache misses"
          value={String(data.cache.misses)}
          note="Includes uncacheable requests"
          icon={<Search size={17} />}
        />
      </div>
      <section className="panel padded">
        <h2>Memory cache</h2>
        <p>
          Answers expire according to their DNS TTL. Least recently used entries
          are removed when capacity is reached. Cache contents are never stored
          in SQLite.
        </p>
        <p>
          Clearing the cache removes all current answers. Lifetime hit and miss
          counters are retained.
        </p>
        <button
          className="button danger"
          disabled={busy}
          onClick={() => void flush()}
        >
          <Eraser size={16} />
          {busy ? "Clearing…" : "Clear cache"}
        </button>
        {message && <p role="status">{message}</p>}
      </section>
    </>
  );
}
