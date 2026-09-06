import { useCallback, useEffect, useState } from "react";
import { loadSnapshot, type Snapshot } from "./api";

export function useSnapshot() {
  const [data, setData] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [history, setHistory] = useState<number[]>([]);
  const refresh = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await loadSnapshot(controller.signal);
        if (controller.signal.aborted) return;
        setData(next);
        setError("");
        setHistory((v) => [...v.slice(-29), next.stats.queries_per_second]);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "Unable to reach server");
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [revision]);
  return { data, error, history, refresh };
}
