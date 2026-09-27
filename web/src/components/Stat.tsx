import type { ReactNode } from "react";

export type Tone = "brand" | "info" | "warning" | "danger";

export function Stat({
  label,
  value,
  note,
  icon,
  tone = "brand",
}: {
  label: string;
  value: string;
  note?: string;
  icon: ReactNode;
  tone?: Tone;
}) {
  return (
    <article className={`stat tone-${tone}`}>
      <div className="stat-heading">
        <span>{label}</span>
        <span className="stat-icon" aria-hidden="true">{icon}</span>
      </div>
      <strong>{value}</strong>
      {note && <small>{note}</small>}
    </article>
  );
}
