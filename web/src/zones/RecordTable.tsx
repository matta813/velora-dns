import { Pencil, Trash2 } from "lucide-react";
import { ownerName, type Zone, type ZoneRecord } from "./types";
import { useI18n } from "../i18n-context";
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
  const { t } = useI18n();
  if (!zone.records.length)
    return (
      <div className="zone-empty">
        <h3>{t("zones.no_records_title")}</h3>
        <p>
          {t("zones.no_records_text")}
        </p>
      </div>
    );
  return (
    <div className="table-wrap">
      <table className="records-table">
        <caption className="sr-only">{t("zones.records_aria")} {zone.name}</caption>
        <thead>
          <tr>
            <th>{t("zones.col_name")}</th>
            <th>{t("zones.col_type")}</th>
            <th>{t("zones.col_value")}</th>
            <th>{t("zones.col_ttl")}</th>
            <th>
              <span className="sr-only">{t("zones.col_actions")}</span>
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
                    aria-label={`${t("zones.edit_aria")} ${ownerName(record.name, zone.name)} ${record.type} ${t("zones.record_word")}`}
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    className="icon-button danger-icon"
                    disabled={disabled}
                    onClick={() => remove(record)}
                    aria-label={`${t("zones.delete_aria")} ${ownerName(record.name, zone.name)} ${record.type} ${t("zones.record_word")}`}
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
