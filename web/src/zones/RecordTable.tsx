import { Pencil, Trash2 } from "lucide-react";
import { ownerName, type Zone, type ZoneRecord } from "./types";
export function RecordTable({
  zone,
  disabled,
  edit,
  remove,
}: {
  zone: Zone;
  disabled: boolean;
  edit: (record: ZoneRecord) => void;
  remove: (record: ZoneRecord) => void;
}) {
  if (!zone.records.length)
    return (
      <div className="zone-empty">
        <h3>No custom records yet</h3>
        <p>
          Add your first record. SOA and fallback apex NS are managed
          automatically.
        </p>
      </div>
    );
  return (
    <div className="table-wrap">
      <table className="records-table">
        <caption className="sr-only">DNS records for {zone.name}</caption>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Value</th>
            <th>TTL</th>
            <th>
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {zone.records.map((record) => (
            <tr key={record.id}>
              <td>
                <code>{ownerName(record.name, zone.name)}</code>
              </td>
              <td>
                <span className="record-type">{record.type}</span>
              </td>
              <td className="record-value">
                <code>
                  {record.type === "MX" ? `${record.priority} ` : ""}
                  {record.value || '""'}
                </code>
              </td>
              <td>{record.ttl}s</td>
              <td>
                <div className="record-actions">
                  <button
                    className="icon-button"
                    disabled={disabled}
                    onClick={() => edit(record)}
                    aria-label={`Edit ${ownerName(record.name, zone.name)} ${record.type} record`}
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    className="icon-button danger-icon"
                    disabled={disabled}
                    onClick={() => remove(record)}
                    aria-label={`Delete ${ownerName(record.name, zone.name)} ${record.type} record`}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
