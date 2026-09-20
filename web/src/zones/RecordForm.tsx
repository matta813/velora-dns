import { useState, type FormEvent } from "react";
import { APIError } from "../api";
import {
  ownerName,
  recordTypes,
  type RecordInput,
  type RecordType,
  type Zone,
  type ZoneRecord,
} from "./types";
import { useI18n } from "../i18n-context";
export function RecordForm({
  zone,
  record,
  busy,
  save,
  cancel,
}: {
  zone: Zone;
  record?: ZoneRecord;
  busy: boolean;
  save: (input: RecordInput) => Promise<void>;
  cancel: () => void;
}) {
  const { t } = useI18n();
  const [name, setName] = useState(
    record ? ownerName(record.name, zone.name) : "",
  );
  const [type, setType] = useState<RecordType>(record?.type ?? "A");
  const [ttl, setTTL] = useState(String(record?.ttl ?? 300));
  const [value, setValue] = useState(record?.value ?? "");
  const [priority, setPriority] = useState(String(record?.priority ?? 10));
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    const lifetime = Number(ttl),
      preference = type === "MX" ? Number(priority) : 0;
    if (
      !ttl.trim() ||
      (type === "MX" && !priority.trim()) ||
      !Number.isInteger(lifetime) ||
      lifetime < 0 ||
      lifetime > 86400 ||
      !Number.isInteger(preference) ||
      preference < 0 ||
      preference > 65535
    ) {
      setError(t("zones.invalid_ttl"));
      return;
    }
    try {
      await save({
        name: name || "@",
        type,
        ttl: lifetime,
        value,
        priority: preference,
      });
    } catch (e) {
      setError(
        e instanceof APIError && e.status === 412
          ? t("zones.record_conflict")
          : e instanceof Error
            ? e.message
            : t("zones.save_failed"),
      );
    }
  }
  const valueLabel =
    type === "A"
      ? t("zones.ipv4_address")
      : type === "AAAA"
        ? t("zones.ipv6_address")
        : type === "TXT"
          ? t("zones.text_value")
          : t("zones.target_hostname");
  return (
    <form
      className="zone-form"
      onSubmit={(event) => void submit(event)}
      aria-label={record ? t("zones.record_form_edit_aria") : t("zones.record_form_add_aria")}
    >
      <div className="form-heading">
        <div>
          <h3>{record ? t("zones.edit_record") : t("zones.add_record_form")}</h3>
          <p>{t("zones.changes_apply")} {zone.name}.</p>
        </div>
      </div>
      <fieldset disabled={busy}>
        <div className="form-grid">
          <label>
            {t("zones.record_name")}
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="router or @"
              autoFocus
              maxLength={253}
              autoComplete="off"
            />
            <small>
              {t("zones.relative_name_hint")}
            </small>
          </label>
          <label>
            {t("zones.record_type")}
            <select
              value={type}
              onChange={(e) => setType(e.target.value as RecordType)}
            >
              {recordTypes.map((t) => (
                <option key={t}>{t}</option>
              ))}
            </select>
          </label>
          <label>
            {t("zones.ttl_seconds")}
            <input
              type="number"
              min={0}
              max={86400}
              required
              value={ttl}
              onChange={(e) => setTTL(e.target.value)}
            />
          </label>
          {type === "MX" && (
            <label>
              {t("zones.priority")}
              <input
                type="number"
                min={0}
                max={65535}
                required
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
              />
            </label>
          )}
        </div>
        <label>
          {valueLabel}
          {type === "TXT" ? (
            <textarea
              value={value}
              onChange={(e) => setValue(e.target.value)}
              maxLength={2048}
              rows={3}
            />
          ) : (
            <input
              required
              value={value}
              onChange={(e) => setValue(e.target.value)}
              maxLength={253}
              autoComplete="off"
              placeholder={
                type === "A"
                  ? "192.168.1.1"
                  : type === "AAAA"
                    ? "2001:db8::1"
                    : "host.home.arpa"
              }
            />
          )}
        </label>
        {error && (
          <p className="notice error" role="alert">
            {error}
          </p>
        )}
        <div className="form-actions">
          <button className="button primary" type="submit">
            {busy ? t("zones.saving") : t("zones.save_record")}
          </button>
          <button className="button" type="button" onClick={cancel}>
            {t("zones.cancel")}
          </button>
        </div>
      </fieldset>
    </form>
  );
}
