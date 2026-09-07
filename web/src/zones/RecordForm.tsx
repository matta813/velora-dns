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
      setError("Enter valid TTL and priority values.");
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
          ? "This zone changed elsewhere. Your draft is unchanged. Close this form and reload zones before retrying."
          : e instanceof Error
            ? e.message
            : "Unable to save record",
      );
    }
  }
  const valueLabel =
    type === "A"
      ? "IPv4 address"
      : type === "AAAA"
        ? "IPv6 address"
        : type === "TXT"
          ? "Text value"
          : "Target hostname";
  return (
    <form
      className="zone-form"
      onSubmit={(event) => void submit(event)}
      aria-label={record ? "Edit DNS record" : "Add DNS record"}
    >
      <div className="form-heading">
        <div>
          <h3>{record ? "Edit record" : "Add record"}</h3>
          <p>Changes apply to {zone.name} after saving.</p>
        </div>
      </div>
      <fieldset disabled={busy}>
        <div className="form-grid">
          <label>
            Record name
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="router or @"
              autoFocus
              maxLength={253}
              autoComplete="off"
            />
            <small>
              Relative name, @ for the zone, or an absolute name ending in a
              dot.
            </small>
          </label>
          <label>
            Record type
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
            TTL (seconds)
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
              Priority
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
            {busy ? "Saving…" : "Save record"}
          </button>
          <button className="button" type="button" onClick={cancel}>
            Cancel
          </button>
        </div>
      </fieldset>
    </form>
  );
}
