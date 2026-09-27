export type Tone = "neutral" | "info" | "danger" | "success" | "warning";

export function Stat({
  label,
  value,
  note,
  aside,
  progress,
  tone = "neutral",
}: {
  label: string;
  value: string;
  note?: string;
  /** Short secondary figure shown next to the label, e.g. a percentage. */
  aside?: string;
  /** 0–1; renders a thin bar along the bottom edge. */
  progress?: number;
  tone?: Tone;
}) {
  return (
    <article className={`stat tone-${tone}`}>
      <div className="stat-heading">
        <span>{label}</span>
        {aside && <span className="stat-aside">{aside}</span>}
      </div>
      <strong>{value}</strong>
      {note && <small>{note}</small>}
      {progress !== undefined && (
        <div className="stat-bar" aria-hidden="true">
          <span style={{ width: `${Math.max(0, Math.min(1, progress)) * 100}%` }} />
        </div>
      )}
    </article>
  );
}
