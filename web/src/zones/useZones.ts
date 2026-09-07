import { useEffect, useRef, useState } from "react";
import { request } from "../api";
import type { RecordInput, Zone, ZoneInput, ZoneRecord } from "./types";
export function useZones() {
  const [zones, setZones] = useState<Zone[] | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    // Defer the state update to the request task, not the effect's render phase.
    void (async () => {
      await Promise.resolve();
      if (controller.signal.aborted) return;
      setLoading(true);
      try {
        const data = await request<Zone[]>("/api/v1/zones", controller.signal);
        if (!controller.signal.aborted) {
          setZones(data);
          setError("");
        }
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : "Unable to load zones");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [revision]);
  async function mutate<T>(operation: () => Promise<T>) {
    setBusy(true);
    try {
      return await operation();
    } finally {
      if (active.current) setBusy(false);
    }
  }
  function publish(zone: Zone) {
    if (active.current)
      setZones((current) =>
        [...(current ?? []).filter((z) => z.id !== zone.id), zone].sort(
          (a, b) => a.name.localeCompare(b.name),
        ),
      );
  }
  async function create(input: ZoneInput) {
    return mutate(async () => {
      const zone = await request<Zone>("/api/v1/zones", undefined, "POST", {
        body: input,
      });
      publish(zone);
      return zone;
    });
  }
  async function saveRecord(
    zone: Zone,
    input: RecordInput,
    record?: ZoneRecord,
  ) {
    return mutate(async () => {
      const updated = await request<Zone>(
        `/api/v1/zones/${zone.id}/records${record ? `/${record.id}` : ""}`,
        undefined,
        record ? "PUT" : "POST",
        { body: input, revision: zone.revision },
      );
      publish(updated);
      return updated;
    });
  }
  async function remove(zone: Zone, record?: ZoneRecord) {
    return mutate(async () => {
      const result = await request<Zone>(
        `/api/v1/zones/${zone.id}${record ? `/records/${record.id}` : ""}`,
        undefined,
        "DELETE",
        { revision: zone.revision },
      );
      if (record) publish(result);
      else if (active.current)
        setZones((current) => (current ?? []).filter((z) => z.id !== zone.id));
    });
  }
  return {
    zones,
    error,
    loading,
    busy,
    create,
    saveRecord,
    remove,
    reload: () => {
      setLoading(true);
      setRevision((v) => v + 1);
    },
  };
}
