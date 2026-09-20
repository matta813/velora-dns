import { useState } from "react";
import { Globe2, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { useZones } from "../zones/useZones";
import { ZoneForm } from "../zones/ZoneForm";
import { RecordForm } from "../zones/RecordForm";
import { RecordTable } from "../zones/RecordTable";
import type { Zone, ZoneRecord } from "../zones/types";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";
export function Zones({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
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
  const filteredZones = state.zones?.filter((z) =>
    z.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const selected =
    filteredZones?.find((z) => z.id === selectedID) ?? filteredZones?.[0];
  const disabled = readOnly || state.busy || state.loading || Boolean(state.error);
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
          ? t("zones.record_deleted")
          : t("zones.zone_deleted"),
      );
      setDeleting(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : t("zones.delete_failed"));
    }
  }
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {state.zones
            ? `${state.zones.length} ${t(state.zones.length === 1 ? "zones.count_one" : "zones.count")}`
            : t("zones.local_authority")}
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
            {t("zones.reload")}
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
            {t("zones.add_zone")}
          </button>
        </div>
      </div>
      {state.error && (
        <div className="notice error" role="alert">
          {state.error}.{" "}
          {state.zones
            ? t("zones.error_stale")
            : t("zones.error_retry")}
        </div>
      )}
      {message && (
        <div className="notice success" role="status">
          {message}
        </div>
      )}
      {actionError && (
        <div className="notice error" role="alert">
          {actionError}. {t("zones.reload_before_retry")}
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
              setMessage(t("zones.zone_created"));
            }}
          />
        </section>
      )}
      {state.zones === null && !state.error && (
        <section className="panel padded" role="status">
          {t("zones.loading")}
        </section>
      )}
      {state.zones?.length === 0 && !creating && (
        <section className="panel zone-empty large">
          <Globe2 size={32} />
          <h2>{t("zones.empty_title")}</h2>
          <p>
            {t("zones.empty_text")}
          </p>
          <button
            className="button primary"
            disabled={disabled}
            onClick={() => setCreating(true)}
          >
            <Plus size={15} />
            {t("zones.create_first")}
          </button>
        </section>
      )}
      {Boolean(state.zones?.length) && (
        <div className="zones-layout">
          <section className="panel zone-list">
            <label className="zone-search">
              <Search size={15} />
              <span className="sr-only">{t("zones.filter_aria")}</span>
              <input
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setEditor(null);
                  setDeleting(null);
                }}
                placeholder={t("zones.filter_placeholder")}
              />
            </label>
            <nav aria-label={t("zones.nav_aria")}>
              {filteredZones?.map((z) => (
                  <button
                    key={z.id}
                    disabled={state.busy}
                    aria-current={selected?.id === z.id ? "true" : undefined}
                    onClick={() => select(z)}
                  >
                    <Globe2 size={17} />
                    <span>
                      <strong>{z.name}</strong>
                      <small>{z.records.length} {t("zones.records_custom")}</small>
                    </span>
                  </button>
                ))}
            </nav>
            {filteredZones?.length === 0 && <p className="padded">{t("zones.no_matching")}</p>}
          </section>
          {selected && (
            <section className="panel zone-detail">
              <div className="panel-heading">
                <div>
                  <span className="eyebrow">{t("zones.authoritative_eyebrow")}</span>
                  <h2>{selected.name}</h2>
                  <p>
                    {t("zones.revision")} {selected.revision} · {selected.records.length}{" "}
                    {t("zones.records_custom")}
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
                    {t("zones.add_record")}
                  </button>
                  <button
                    className="icon-button danger-icon"
                    disabled={disabled || Boolean(editor)}
                    aria-label={t("zones.delete_zone_aria")}
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
                  {t("zones.primary_ns")} <code>{selected.primary_ns}</code>
                </span>
                <span>
                  {t("zones.contact")} <code>{selected.contact}</code>
                </span>
              </div>
              {deleting && (
                <div className="notice zone-confirm" role="alert">
                  <p>
                    {deleting.record ? (
                      <>
                        {t("zones.delete_record_confirm")} <strong>{deleting.record.name}</strong> (
                        {deleting.record.type})?
                      </>
                    ) : (
                      <>
                        {t("zones.delete_zone_confirm")} <strong>{deleting.zone.name}</strong> {" "}
                        {deleting.zone.records.length} {t("zones.delete_zone_confirm2")}
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
                        ? t("zones.deleting")
                        : deleting.record
                          ? t("zones.delete_record_perm")
                          : t("zones.delete_zone_perm")}
                    </button>
                    <button
                      className="button"
                      disabled={state.busy}
                      onClick={() => setDeleting(null)}
                    >
                      {t("zones.cancel_deletion")}
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
                    setMessage(t("zones.record_saved"));
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
              <p className="panel-footnote">{t("zones.footnote")}</p>
            </section>
          )}
        </div>
      )}
    </>
  );
}
