import type { ReactNode } from "react";
import type { BadgeTone } from "./tone";

export function Badge({
  tone = "neutral",
  plain = false,
  children,
}: {
  tone?: BadgeTone;
  plain?: boolean;
  children: ReactNode;
}) {
  return <span className={`badge ${tone}${plain ? " plain" : ""}`}>{children}</span>;
}
