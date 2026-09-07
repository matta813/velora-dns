import { useState } from "react";
import { Globe2, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { useZones } from "../zones/useZones";
import { ZoneForm } from "../zones/ZoneForm";
import { RecordForm } from "../zones/RecordForm";
import { RecordTable } from "../zones/RecordTable";
import type { Zone, ZoneRecord } from "../zones/types";
import "../zones/zones.css";
export function Zones() {
  const state = useZones();
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [creating, setCreating] = useState(false);
  const [editor, setEditor] = useState<{
    zone: Zone;
    record?: ZoneRecord;
  } | null>(null);
  const [deleting, setDeleting] = useState<{
    zone: Zone;
    record?: ZoneRecord;
  } | null>(null);
  const [query, setQuery] = useState("");
  const [message, setMessage] = useState("");
  const [actionError, setActionError] = useState("");
  const selected =
    state.zones?.find((z) => z.id === selectedID) ?? state.zones?.[0];
  const disabled = state.busy || state.loading || Boolean(state.error);
  function select(zone: Zone) {
    setSelectedID(zone.id);
    setEditor(null);
    setDeleting(null);
    setMessage("");
    setActionError("");
  }
  async function remove() {
    if (!deleting) return;
    setActionError("");
    try {
      await state.remove(deleting.zone, deleting.record);
      setMessage(
        deleting.record
          ? "Record deleted. DNS changes are active."
          : "Zone and its records deleted.",
      );
      setDeleting(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Unable to delete");
    }
  }
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {state.zones
            ? `${state.zones.length} local ${state.zones.length === 1 ? "zone" : "zones"}`
            : "Local authority"}
        </span>
        <div>
          <button
            className="button"
            disabled={
              state.busy || state.loading || Boolean(editor) || creating
            }
            onClick={() => {
              state.reload();
              setDeleting(null);
              setActionError("");
              setMessage("");
            }}
          >
            <RefreshCw size={15} />
            Reload zones
          </button>
          <button
            className="button primary"
            disabled={disabled}
            onClick={() => {
              setCreating(true);
              setEditor(null);
              setDeleting(null);
            }}
          >
            <Plus size={15} />
            Add zone
          </button>
        </div>
      </div>
      {state.error && (
        <div className="notice error" role="alert">
          {state.error}.{" "}
          {state.zones
            ? "Showing the last loaded zones. Reload before making changes."
            : "Reload to try again."}
        </div>
      )}
      {message && (
        <div className="notice success" role="status">
          {message}
        </div>
      )}
      {actionError && (
        <div className="notice error" role="alert">
          {actionError}. Reload zones before retrying.
        </div>
      )}
      {creating && (
        <section className="panel form-panel">
          <ZoneForm
            busy={disabled}
            cancel={() => setCreating(false)}
            save={async (input) => {
              const z = await state.create(input);
              setSelectedID(z.id);
              setCreating(false);
              setMessage("Zone created. Add records to start using it.");
            }}
          />
        </section>
      )}
      {state.zones === null && !state.error && (
        <section className="panel padded" role="status">
          Loading local zones…
        </section>
      )}
      {state.zones?.length === 0 && !creating && (
        <section className="panel zone-empty large">
          <Globe2 size={32} />
          <h2>Give your network familiar names</h2>
          <p>
            Create a zone such as home.arpa, then add records for your router,
            services, and devices.
          </p>
          <button
            className="button primary"
            disabled={disabled}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} />
            Create your first zone
          </button>
        </section>
      )}
      {Boolean(state.zones?.length) && (
        <div className="zones-layout">
          <section className="panel zone-list">
            <label className="zone-search">
              <Search size={15} />
              <span className="sr-only">Filter zones</span>
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Find a zone…"
              />
            </label>
            <nav aria-label="Local zones">
              {state.zones
                ?.filter((z) => z.name.includes(query.toLowerCase()))
                .map((z) => (
                  <button
                    key={z.id}
                    disabled={state.busy}
                    aria-current={selected?.id === z.id ? "true" : undefined}
                    onClick={() => select(z)}
                  >
                    <Globe2 size={17} />
                    <span>
                      <strong>{z.name}</strong>
                      <small>{z.records.length} custom records</small>
                    </span>
                  </button>
                ))}
            </nav>
            {state.zones?.every(
              (z) => !z.name.includes(query.toLowerCase()),
            ) && <p className="padded">No matching zones.</p>}
          </section>
          {selected && (
            <section className="panel zone-detail">
              <div className="panel-heading">
                <div>
                  <span className="eyebrow">AUTHORITATIVE ZONE</span>
                  <h2>{selected.name}</h2>
                  <p>
                    Revision {selected.revision} · {selected.records.length}{" "}
                    custom records
                  </p>
                </div>
                <div className="zone-detail-actions">
                  <button
                    className="button"
                    disabled={disabled || Boolean(editor)}
                    onClick={() => {
                      setEditor({ zone: selected });
                      setDeleting(null);
                      setActionError("");
                    }}
                  >
                    <Plus size={14} />
                    Add record
                  </button>
                  <button
                    className="icon-button danger-icon"
                    disabled={disabled || Boolean(editor)}
                    aria-label="Delete zone"
                    onClick={() => {
                      setDeleting({ zone: selected });
                      setActionError("");
                    }}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
              <div className="zone-authority">
                <span>
                  Primary NS <code>{selected.primary_ns}</code>
                </span>
                <span>
                  Contact <code>{selected.contact}</code>
                </span>
              </div>
              {deleting && (
                <div className="notice zone-confirm" role="alert">
                  <p>
                    {deleting.record ? (
                      <>
                        Delete <strong>{deleting.record.name}</strong> (
                        {deleting.record.type})?
                      </>
                    ) : (
                      <>
                        Delete <strong>{deleting.zone.name}</strong> and all{" "}
                        {deleting.zone.records.length} records? This cannot be
                        undone.
                      </>
                    )}
                  </p>
                  <div className="form-actions">
                    <button
                      className="button danger"
                      disabled={disabled}
                      onClick={() => void remove()}
                    >
                      {state.busy
                        ? "Deleting…"
                        : deleting.record
                          ? "Delete record permanently"
                          : "Delete zone permanently"}
                    </button>
                    <button
                      className="button"
                      disabled={state.busy}
                      onClick={() => setDeleting(null)}
                    >
                      Cancel deletion
                    </button>
                  </div>
                </div>
              )}
              {editor && (
                <RecordForm
                  key={`${editor.zone.id}-${editor.record?.id ?? "new"}`}
                  zone={editor.zone}
                  record={editor.record}
                  busy={disabled}
                  cancel={() => setEditor(null)}
                  save={async (input) => {
                    await state.saveRecord(editor.zone, input, editor.record);
                    setEditor(null);
                    setMessage("Record saved. DNS changes are active.");
                  }}
                />
              )}
              <RecordTable
                zone={selected}
                disabled={disabled || Boolean(editor)}
                edit={(record) => {
                  setEditor({ zone: selected, record });
                  setDeleting(null);
                  setActionError("");
                }}
                remove={(record) => {
                  setDeleting({ zone: selected, record });
                  setActionError("");
                }}
              />
              <p className="panel-footnote">
                Local answers take priority over forwarding. Devices may retain
                previous answers until their TTL expires.
              </p>
            </section>
          )}
        </div>
      )}
    </>
  );
}
